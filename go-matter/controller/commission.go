package controller

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"fmt"

	"github.com/sh5080/go-matter/cert"
	"github.com/sh5080/go-matter/cluster"
	"github.com/sh5080/go-matter/im"
	"github.com/sh5080/go-matter/message"
	"github.com/sh5080/go-matter/pase"
	"github.com/sh5080/go-matter/transport"
)

// failSafeExpirySeconds bounds how long the device waits for the flow to
// finish before reverting (re-armed implicitly by each successful step).
const failSafeExpirySeconds = 60

// Commission onboards the device reachable over t onto the fabric in store,
// assigning it newNodeID (Spec 5.5, on-network flow):
//
//	PASE (passcode) → ArmFailSafe → CSRRequest → AddTrustedRootCertificate
//	→ AddNOC(newNodeID)
//
// After it returns, the device is on the fabric but the fail-safe is still
// armed: re-connect over CASE (operational discovery) and send
// CommissioningComplete — see CompleteCommissioning.
//
// Device attestation (DAC chain verification) is NOT performed: this
// controller trusts the device it was pointed at, which is acceptable for a
// hub commissioning its owner's devices on a private LAN. Add CSA
// root-store-based verification before trusting unknown hardware.
func Commission(ctx context.Context, t transport.Transport, passcode uint32, store StoredFabric, newNodeID uint64, adminVendorID uint16) error {
	if len(store.RootKey) == 0 {
		return fmt.Errorf("controller: fabric cannot issue NOCs (no CA key); generate the fabric with GenerateFabric")
	}
	sess, err := paseHandshake(ctx, t, passcode)
	if err != nil {
		return fmt.Errorf("controller: PASE: %w", err)
	}

	// Arm the fail-safe so a failed flow reverts on the device side.
	arm, err := cluster.ArmFailSafe(failSafeExpirySeconds)
	if err != nil {
		return err
	}
	if err := invokeCommissioning(ctx, sess, arm, cluster.CmdArmFailSafeResponse); err != nil {
		return fmt.Errorf("controller: ArmFailSafe: %w", err)
	}

	// The device generates its operational keypair and proves possession via
	// a CSR; we issue its NOC from the fabric CA.
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	csrReq, err := cluster.CSRRequest(nonce)
	if err != nil {
		return err
	}
	res, err := sess.Invoke(ctx, csrReq)
	if err != nil {
		return fmt.Errorf("controller: CSRRequest: %w", err)
	}
	if res.Command == nil || res.Command.Path.Command != cluster.CmdCSRResponse {
		return fmt.Errorf("controller: expected CSRResponse, got %+v", res)
	}
	csrResp, err := cluster.DecodeCSRResponse(res.Command.Fields)
	if err != nil {
		return err
	}
	csrDER, echoedNonce, err := cluster.ParseNOCSRElements(csrResp.NOCSRElements)
	if err != nil {
		return err
	}
	if !bytes.Equal(echoedNonce, nonce) {
		return fmt.Errorf("controller: CSR nonce mismatch")
	}
	devicePub, err := publicKeyFromCSR(csrDER)
	if err != nil {
		return err
	}
	noc, err := store.IssueNOC(devicePub, newNodeID)
	if err != nil {
		return fmt.Errorf("controller: issue NOC: %w", err)
	}

	addRoot, err := cluster.AddTrustedRootCert(store.RCAC)
	if err != nil {
		return err
	}
	if res, err = sess.Invoke(ctx, addRoot); err != nil {
		return fmt.Errorf("controller: AddTrustedRootCertificate: %w", err)
	}
	if res.Status != nil && res.Status.Status != im.StatusSuccess {
		return fmt.Errorf("controller: AddTrustedRootCertificate rejected: 0x%02x", res.Status.Status)
	}

	addNOC, err := cluster.AddNOC(noc, store.IPK, controllerNodeID(store), adminVendorID)
	if err != nil {
		return err
	}
	if res, err = sess.Invoke(ctx, addNOC); err != nil {
		return fmt.Errorf("controller: AddNOC: %w", err)
	}
	if res.Command == nil || res.Command.Path.Command != cluster.CmdNOCResponse {
		return fmt.Errorf("controller: expected NOCResponse, got %+v", res)
	}
	nocResp, err := cluster.DecodeNOCResponse(res.Command.Fields)
	if err != nil {
		return err
	}
	if nocResp.Status != 0 {
		return fmt.Errorf("controller: AddNOC failed with status %d", nocResp.Status)
	}
	return nil
}

// CompleteCommissioning sends CommissioningComplete over an established CASE
// session on the new fabric, disarming the device's fail-safe (Spec 5.5: this
// step MUST run over CASE, not PASE).
func CompleteCommissioning(ctx context.Context, sess *Session) error {
	res, err := sess.Invoke(ctx, cluster.CommissioningComplete())
	if err != nil {
		return fmt.Errorf("controller: CommissioningComplete: %w", err)
	}
	if res.Command != nil && res.Command.Path.Command == cluster.CmdCommissioningCompleteResponse {
		code, err := cluster.DecodeCommissioningError(res.Command.Fields)
		if err != nil {
			return err
		}
		if code != 0 {
			return fmt.Errorf("controller: CommissioningComplete error code %d", code)
		}
		return nil
	}
	if res.Status != nil && res.Status.Status != im.StatusSuccess {
		return fmt.Errorf("controller: CommissioningComplete rejected: 0x%02x", res.Status.Status)
	}
	return nil
}

// controllerNodeID reads the controller's node id from its NOC (the AddNOC
// caseAdminSubject: who may administer the device over CASE).
func controllerNodeID(store StoredFabric) uint64 {
	noc, err := cert.Decode(store.ControllerNOC)
	if err != nil {
		return 0
	}
	id, _ := noc.Subject.NodeID()
	return id
}

// paseHandshake runs PASE over t with MRP (retransmission + acks), returning
// an IM-capable session keyed by the passcode.
func paseHandshake(ctx context.Context, t transport.Transport, passcode uint32) (*Session, error) {
	// Session and exchange ids for the unsecured PASE conversation.
	const localSessionID = 1
	const exchangeID = 1

	in := pase.NewInitiator(passcode, localSessionID)
	seen := make(map[uint32]bool)

	req, err := in.ParamRequest()
	if err != nil {
		return nil, err
	}
	reqFrame := frameUnsecured(0, exchangeID, true, message.SCPBKDFParamRequest, req)
	if err := t.Send(reqFrame); err != nil {
		return nil, err
	}
	hdr, ph, payload, err := awaitSC(ctx, t, reqFrame, seen)
	if err != nil {
		return nil, err
	}
	if ph.Opcode == message.SCStatusReport {
		return nil, statusReportError(payload)
	}
	if ph.Opcode != message.SCPBKDFParamResponse {
		return nil, fmt.Errorf("expected PBKDFParamResponse, got opcode 0x%02x", ph.Opcode)
	}

	pake1, err := in.HandleParamResponse(payload)
	if err != nil {
		return nil, err
	}
	pake1Frame := frameUnsecuredAck(1, exchangeID, true, message.SCPASEPake1, pake1, hdr.Counter, true)
	if err := t.Send(pake1Frame); err != nil {
		return nil, err
	}
	hdr, ph, payload, err = awaitSC(ctx, t, pake1Frame, seen)
	if err != nil {
		return nil, err
	}
	if ph.Opcode == message.SCStatusReport {
		return nil, statusReportError(payload)
	}
	if ph.Opcode != message.SCPASEPake2 {
		return nil, fmt.Errorf("expected Pake2, got opcode 0x%02x", ph.Opcode)
	}

	pake3, err := in.HandlePake2(payload)
	if err != nil {
		return nil, err
	}
	pake3Frame := frameUnsecuredAck(2, exchangeID, true, message.SCPASEPake3, pake3, hdr.Counter, true)
	if err := t.Send(pake3Frame); err != nil {
		return nil, err
	}
	hdr, ph, payload, err = awaitSC(ctx, t, pake3Frame, seen)
	if err != nil {
		return nil, err
	}
	if ph.Opcode != message.SCStatusReport {
		return nil, fmt.Errorf("expected StatusReport, got opcode 0x%02x", ph.Opcode)
	}
	if err := statusReportError(payload); err != nil {
		return nil, err
	}
	_ = t.Send(frameUnsecuredAck(3, exchangeID, true, message.SCStandaloneAck, nil, hdr.Counter, true))

	secure, err := in.SecureSession()
	if err != nil {
		return nil, err
	}
	return &Session{secure: secure, t: t, exchange: exchangeID + 1}, nil
}

// invokeCommissioning sends cmd and accepts either a bare SUCCESS status or a
// General Commissioning response command with errorCode 0.
func invokeCommissioning(ctx context.Context, sess *Session, cmd im.InvokeCommand, wantResponse uint32) error {
	res, err := sess.Invoke(ctx, cmd)
	if err != nil {
		return err
	}
	if res.Command != nil && res.Command.Path.Command == wantResponse {
		code, err := cluster.DecodeCommissioningError(res.Command.Fields)
		if err != nil {
			return err
		}
		if code != 0 {
			return fmt.Errorf("device reported commissioning error %d", code)
		}
		return nil
	}
	if res.Status != nil && res.Status.Status != im.StatusSuccess {
		return fmt.Errorf("rejected with IM status 0x%02x", res.Status.Status)
	}
	return nil
}

// publicKeyFromCSR extracts the uncompressed P-256 public key from a DER
// PKCS#10 CSR, verifying its self-signature (proof of key possession).
func publicKeyFromCSR(der []byte) ([]byte, error) {
	csr, err := x509.ParseCertificateRequest(der)
	if err != nil {
		return nil, fmt.Errorf("parse CSR: %w", err)
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("CSR signature: %w", err)
	}
	pk, ok := csr.PublicKey.(*ecdsa.PublicKey)
	if !ok || pk.Curve.Params().Name != "P-256" {
		return nil, fmt.Errorf("CSR key is not P-256 ECDSA")
	}
	out := make([]byte, 65)
	out[0] = 0x04
	pk.X.FillBytes(out[1:33])
	pk.Y.FillBytes(out[33:])
	return out, nil
}
