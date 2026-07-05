package controller

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"testing"
	"time"

	"github.com/sh5080/go-matter/casesession"
	"github.com/sh5080/go-matter/cert"
	"github.com/sh5080/go-matter/cluster"
	"github.com/sh5080/go-matter/im"
	"github.com/sh5080/go-matter/message"
	"github.com/sh5080/go-matter/pase"
	"github.com/sh5080/go-matter/session"
	"github.com/sh5080/go-matter/tlv"
	"github.com/sh5080/go-matter/transport"
)

const commissioneePasscode = uint32(20202021)

// commissionee is a fake device that starts factory-fresh (passcode only) and
// derives ALL fabric credentials from the commissioning flow itself, then
// serves CASE + OnOff with them — proving the controller-issued credentials
// actually work.
type commissionee struct {
	t  *testing.T
	tp transport.Transport

	// factory state
	passcode uint32
	params   pase.PBKDFParams

	// operational key (created at CSRRequest)
	opKey []byte

	// credentials received during commissioning
	rcacTLV []byte
	nocTLV  []byte
	ipk     []byte

	failSafeArmed bool
	complete      bool
	on            bool // OnOff cluster state
}

// run drives the device through PASE → commissioning → CASE → operational.
func (d *commissionee) run(ctx context.Context) {
	rd, err := pase.NewResponder(d.passcode, d.params, 0x0100)
	if err != nil {
		d.t.Errorf("commissionee: %v", err)
		return
	}

	// --- PASE handshake (unsecured frames) ---
	recvUnsec := func() (message.Header, message.ProtoHeader, []byte, bool) {
		for {
			f, err := d.tp.Receive(ctx)
			if err != nil {
				return message.Header{}, message.ProtoHeader{}, nil, false
			}
			hdr, ph, payload, perr := parseUnsecuredMsg(f)
			if perr != nil {
				continue
			}
			if isStandaloneAck(ph) {
				continue
			}
			return hdr, ph, payload, true
		}
	}

	hdr, ph, payload, ok := recvUnsec()
	if !ok {
		return // context cancelled / transport closed
	}
	if ph.Opcode != message.SCPBKDFParamRequest {
		d.t.Errorf("commissionee: expected PBKDFParamRequest, got 0x%02x", ph.Opcode)
		return
	}
	exch := ph.ExchangeID
	resp, err := rd.HandleParamRequest(payload)
	if err != nil {
		d.t.Errorf("commissionee: param request: %v", err)
		return
	}
	d.tp.Send(frameUnsecuredAck(0, exch, false, message.SCPBKDFParamResponse, resp, hdr.Counter, true))

	hdr, ph, payload, ok = recvUnsec()
	if !ok {
		return
	}
	if ph.Opcode != message.SCPASEPake1 {
		d.t.Errorf("commissionee: expected Pake1, got 0x%02x", ph.Opcode)
		return
	}
	pake2, err := rd.HandlePake1(payload)
	if err != nil {
		d.t.Errorf("commissionee: pake1: %v", err)
		return
	}
	d.tp.Send(frameUnsecuredAck(1, exch, false, message.SCPASEPake2, pake2, hdr.Counter, true))

	hdr, ph, payload, ok = recvUnsec()
	if !ok {
		return
	}
	if ph.Opcode != message.SCPASEPake3 {
		d.t.Errorf("commissionee: expected Pake3, got 0x%02x", ph.Opcode)
		return
	}
	if err := rd.HandlePake3(payload); err != nil {
		d.t.Errorf("commissionee: pake3: %v", err)
		return
	}
	sr := message.StatusReport{GeneralCode: message.GeneralSuccess, ProtocolID: uint32(message.ProtocolSecureChannel)}
	d.tp.Send(frameUnsecuredAck(2, exch, false, message.SCStatusReport, sr.Encode(), hdr.Counter, true))

	secure, err := rd.SecureSession()
	if err != nil {
		d.t.Errorf("commissionee: pase session: %v", err)
		return
	}

	// --- Commissioning commands over the PASE session ---
	var caseFirstFrame []byte
	for caseFirstFrame == nil {
		f, err := d.tp.Receive(ctx)
		if err != nil {
			return
		}
		if isUnsecuredFrame(f) {
			// Either a PASE-time standalone ack or the first CASE Sigma1.
			if _, p, _, perr := parseUnsecuredMsg(f); perr == nil && !isStandaloneAck(p) {
				caseFirstFrame = f
			}
			continue
		}
		counter, payload, derr := secure.DecryptMsg(f)
		if derr != nil {
			continue // duplicate or foreign frame; loopback needs no re-ack
		}
		p, imBytes, err := message.DecodeProto(payload)
		if err != nil || isStandaloneAck(p) {
			continue
		}
		if p.Opcode != message.IMInvokeRequest {
			continue
		}
		cmds, _, _, _ := im.DecodeInvokeRequest(imBytes)
		result := d.handleCommissioningCmd(cmds[0])
		respIM, _ := im.EncodeInvokeResponse([]im.InvokeResult{result}, false)
		rp := message.ProtoHeader{
			Initiator: false, Reliable: true, Opcode: message.IMInvokeResponse,
			ExchangeID: p.ExchangeID, ProtocolID: message.ProtocolInteractionModel,
			AckPresent: true, AckCounter: counter,
		}
		frame, _ := secure.Encrypt(append(rp.Encode(), respIM...))
		d.tp.Send(frame)
	}

	// --- CASE with the credentials received above ---
	rcac, err := cert.Decode(d.rcacTLV)
	if err != nil {
		d.t.Errorf("commissionee: decode received RCAC: %v", err)
		return
	}
	noc, err := cert.Decode(d.nocTLV)
	if err != nil {
		d.t.Errorf("commissionee: decode received NOC: %v", err)
		return
	}
	fabricID, _ := noc.Subject.FabricID()
	fabric := casesession.Fabric{IPK: d.ipk, FabricID: fabricID, RootPubKey: rcac.PublicKey, RCAC: rcac}
	identity := casesession.Identity{NOC: d.nocTLV, OpKey: d.opKey}

	responder, err := casesession.NewResponder(fabric, identity, 0x0200)
	if err != nil {
		d.t.Errorf("commissionee: case responder: %v", err)
		return
	}
	p1, sigma1, err := parseUnsecured(caseFirstFrame)
	if err != nil || p1.Opcode != message.SCCASESigma1 {
		d.t.Errorf("commissionee: expected Sigma1 (%v, opcode 0x%02x)", err, p1.Opcode)
		return
	}
	sigma2, err := responder.HandleSigma1(sigma1)
	if err != nil {
		d.t.Errorf("commissionee: sigma1: %v", err)
		return
	}
	d.tp.Send(frameUnsecured(3, p1.ExchangeID, false, message.SCCASESigma2, sigma2))

	f3, err := d.tp.Receive(ctx)
	if err != nil {
		return
	}
	p3, sigma3, err := parseUnsecured(f3)
	if err != nil || p3.Opcode != message.SCCASESigma3 {
		d.t.Errorf("commissionee: expected Sigma3")
		return
	}
	if err := responder.HandleSigma3(sigma3); err != nil {
		d.t.Errorf("commissionee: sigma3: %v", err)
		return
	}
	d.tp.Send(frameUnsecured(4, p3.ExchangeID, false, message.SCStatusReport, sr.Encode()))

	caseSecure, err := responder.SecureSession()
	if err != nil {
		d.t.Errorf("commissionee: case session: %v", err)
		return
	}

	// --- Operational phase over CASE ---
	for {
		f, err := d.tp.Receive(ctx)
		if err != nil {
			return
		}
		if isUnsecuredFrame(f) {
			continue
		}
		counter, payload, derr := caseSecure.DecryptMsg(f)
		if derr != nil {
			continue
		}
		p, imBytes, err := message.DecodeProto(payload)
		if err != nil || isStandaloneAck(p) {
			continue
		}
		if p.Opcode != message.IMInvokeRequest {
			continue
		}
		cmds, _, _, _ := im.DecodeInvokeRequest(imBytes)
		result := d.handleOperationalCmd(cmds[0])
		respIM, _ := im.EncodeInvokeResponse([]im.InvokeResult{result}, false)
		rp := message.ProtoHeader{
			Initiator: false, Reliable: true, Opcode: message.IMInvokeResponse,
			ExchangeID: p.ExchangeID, ProtocolID: message.ProtocolInteractionModel,
			AckPresent: true, AckCounter: counter,
		}
		frame, _ := caseSecure.Encrypt(append(rp.Encode(), respIM...))
		d.tp.Send(frame)
	}
}

// handleCommissioningCmd implements the minimum commissioning cluster surface.
func (d *commissionee) handleCommissioningCmd(c im.InvokeCommand) im.InvokeResult {
	success := im.InvokeResult{Status: &im.CommandStatus{Path: c.Path, Status: im.StatusSuccess}}
	respCmd := func(command uint32, fields []byte) im.InvokeResult {
		return im.InvokeResult{Command: &im.InvokeCommand{
			Path:   im.CommandPath{Endpoint: 0, Cluster: c.Path.Cluster, Command: command},
			Fields: fields,
		}}
	}
	gcError := func(code uint8) []byte {
		w := tlv.NewWriter()
		w.PutUint(tlv.Context(0), uint64(code))
		w.PutString(tlv.Context(1), "")
		b, _ := w.Bytes()
		return b
	}

	switch {
	case c.Path.Cluster == cluster.GeneralCommissioningID && c.Path.Command == 0x00: // ArmFailSafe
		d.failSafeArmed = true
		return respCmd(cluster.CmdArmFailSafeResponse, gcError(0))

	case c.Path.Cluster == cluster.OperationalCredentialsID && c.Path.Command == 0x04: // CSRRequest
		var nonce []byte
		r := tlv.NewReader(c.Fields)
		for r.Next() {
			if r.Tag().Num == 0 {
				nonce, _ = r.Bytes()
			}
		}
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			d.t.Errorf("commissionee: keygen: %v", err)
			return success
		}
		d.opKey = make([]byte, 32)
		key.D.FillBytes(d.opKey)
		csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
		if err != nil {
			d.t.Errorf("commissionee: CSR: %v", err)
			return success
		}
		elements, _ := cluster.EncodeNOCSRElements(csrDER, nonce)
		w := tlv.NewWriter()
		w.PutBytes(tlv.Context(0), elements)
		w.PutBytes(tlv.Context(1), bytes.Repeat([]byte{0xEE}, 64)) // attestation sig (unverified in v1)
		fields, _ := w.Bytes()
		return respCmd(cluster.CmdCSRResponse, fields)

	case c.Path.Cluster == cluster.OperationalCredentialsID && c.Path.Command == 0x0B: // AddTrustedRootCertificate
		r := tlv.NewReader(c.Fields)
		for r.Next() {
			if r.Tag().Num == 0 {
				d.rcacTLV, _ = r.Bytes()
			}
		}
		return success

	case c.Path.Cluster == cluster.OperationalCredentialsID && c.Path.Command == 0x06: // AddNOC
		r := tlv.NewReader(c.Fields)
		for r.Next() {
			switch r.Tag().Num {
			case 0:
				d.nocTLV, _ = r.Bytes()
			case 2:
				d.ipk, _ = r.Bytes()
			}
		}
		w := tlv.NewWriter()
		w.PutUint(tlv.Context(0), 0) // status OK
		w.PutUint(tlv.Context(1), 1) // fabric index
		fields, _ := w.Bytes()
		return respCmd(cluster.CmdNOCResponse, fields)
	}
	return success
}

// handleOperationalCmd serves CommissioningComplete and OnOff over CASE.
func (d *commissionee) handleOperationalCmd(c im.InvokeCommand) im.InvokeResult {
	success := im.InvokeResult{Status: &im.CommandStatus{Path: c.Path, Status: im.StatusSuccess}}
	switch {
	case c.Path.Cluster == cluster.GeneralCommissioningID && c.Path.Command == 0x04: // CommissioningComplete
		d.failSafeArmed = false
		d.complete = true
		w := tlv.NewWriter()
		w.PutUint(tlv.Context(0), 0)
		w.PutString(tlv.Context(1), "")
		fields, _ := w.Bytes()
		return im.InvokeResult{Command: &im.InvokeCommand{
			Path:   im.CommandPath{Endpoint: 0, Cluster: cluster.GeneralCommissioningID, Command: cluster.CmdCommissioningCompleteResponse},
			Fields: fields,
		}}
	case c.Path.Cluster == 0x0006: // OnOff
		switch c.Path.Command {
		case 0x00:
			d.on = false
		case 0x01:
			d.on = true
		}
		return success
	}
	return success
}

// TestCommissionEndToEnd commissions a factory-fresh device onto a freshly
// generated fabric, reconnects over CASE with the issued credentials, sends
// CommissioningComplete, and drives an OnOff command — the full self-contained
// onboarding path with no external tools.
func TestCommissionEndToEnd(t *testing.T) {
	store, err := GenerateFabric(0xFAB0000000000001, 0x1111000000000001)
	if err != nil {
		t.Fatalf("generate fabric: %v", err)
	}

	ctrlPipe, devPipe := transport.NewPipe()
	defer ctrlPipe.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	dev := &commissionee{
		t: t, tp: devPipe,
		passcode: commissioneePasscode,
		params:   pase.PBKDFParams{Iterations: 1000, Salt: bytes.Repeat([]byte{0x5A}, 16)},
	}
	go dev.run(ctx)

	const newNodeID = uint64(0x2222000000000042)
	if err := Commission(ctx, ctrlPipe, commissioneePasscode, store, newNodeID, 0xFFF1); err != nil {
		t.Fatalf("commission: %v", err)
	}

	// Reconnect over CASE with the new credentials, as production would after
	// operational mDNS discovery.
	fabric, err := store.Fabric()
	if err != nil {
		t.Fatal(err)
	}
	sess, err := New(fabric, store.Identity()).Connect(ctx, ctrlPipe, newNodeID)
	if err != nil {
		t.Fatalf("CASE after commissioning: %v", err)
	}
	if err := CompleteCommissioning(ctx, sess); err != nil {
		t.Fatalf("complete: %v", err)
	}

	// Drive the device operationally.
	res, err := sess.Invoke(ctx, cluster.OnOffOn(1))
	if err != nil || res.Status == nil || res.Status.Status != im.StatusSuccess {
		t.Fatalf("post-commissioning invoke: %+v (%v)", res, err)
	}

	if !dev.complete {
		t.Fatal("device never saw CommissioningComplete")
	}
	if dev.failSafeArmed {
		t.Fatal("fail-safe still armed after completion")
	}
	if !dev.on {
		t.Fatal("OnOff command did not reach the device")
	}
}

// TestCommissionWrongPasscode ensures a bad passcode fails at PASE, before any
// credentials move.
func TestCommissionWrongPasscode(t *testing.T) {
	store, err := GenerateFabric(0xFAB0000000000001, 0x1111000000000001)
	if err != nil {
		t.Fatal(err)
	}
	ctrlPipe, devPipe := transport.NewPipe()
	defer ctrlPipe.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dev := &commissionee{
		t: t, tp: devPipe,
		passcode: commissioneePasscode,
		params:   pase.PBKDFParams{Iterations: 1000, Salt: bytes.Repeat([]byte{0x5A}, 16)},
	}
	go dev.run(ctx)

	err = Commission(ctx, ctrlPipe, commissioneePasscode+1, store, 0x42, 0xFFF1)
	if err == nil {
		t.Fatal("commissioning with a wrong passcode must fail")
	}
	if dev.nocTLV != nil {
		t.Fatal("no credentials may be installed after a failed PASE")
	}
}

// TestGenerateFabricSelfConsistent checks the generated fabric verifies its
// own certificate chain and round-trips through the store.
func TestGenerateFabricSelfConsistent(t *testing.T) {
	store, err := GenerateFabric(0xFAB0000000000002, 0x1111000000000002)
	if err != nil {
		t.Fatal(err)
	}
	fabric, err := store.Fabric()
	if err != nil {
		t.Fatal(err)
	}
	if fabric.FabricID != 0xFAB0000000000002 {
		t.Fatalf("fabric id = %#x", fabric.FabricID)
	}
	noc, err := cert.Decode(store.ControllerNOC)
	if err != nil {
		t.Fatal(err)
	}
	if node, ok := noc.Subject.NodeID(); !ok || node != 0x1111000000000002 {
		t.Fatalf("controller node id = %#x (%v)", node, ok)
	}
	// The controller NOC must chain to the RCAC (no ICAC in this fabric).
	if err := cert.VerifyChain(noc, nil, fabric.RCAC); err != nil {
		t.Fatalf("NOC does not verify against RCAC: %v", err)
	}
}

var _ = session.Secure{} // keep the session import when helpers change
