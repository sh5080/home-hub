package controller

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
	"time"

	"github.com/sh5080/go-matter/casesession"
	"github.com/sh5080/go-matter/cert"
	"github.com/sh5080/go-matter/cluster"
	"github.com/sh5080/go-matter/crypto"
	"github.com/sh5080/go-matter/im"
	"github.com/sh5080/go-matter/message"
	"github.com/sh5080/go-matter/tlv"
	"github.com/sh5080/go-matter/transport"
)

const (
	fabricID = 0xFAB000000000001D
	rootID   = 0xCACACACA00000001
	ctrlNode = 0x1111000000000001
	devNode  = 0x2222000000000002
)

func u16p(v uint16) *uint16 { return &v }
func u8p(v uint8) *uint8     { return &v }

func genKey(t *testing.T) (scalar, pub []byte) {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	scalar = make([]byte, 32)
	k.D.FillBytes(scalar)
	if pub, err = crypto.PublicFromScalar(scalar); err != nil {
		t.Fatal(err)
	}
	return
}

func noc(pub []byte, node uint64) *cert.Cert {
	return &cert.Cert{
		SerialNumber: []byte{0x02}, SigAlgo: 1,
		Issuer:    cert.DN{Attrs: []cert.Attr{{Tag: cert.DNMatterRCACID, Value: rootID}}},
		NotBefore: 0x271b17ef, NotAfter: 0x4cb9b56e,
		Subject: cert.DN{Attrs: []cert.Attr{
			{Tag: cert.DNMatterNodeID, Value: node},
			{Tag: cert.DNMatterFabricID, Value: fabricID},
		}},
		PubKeyAlgo: 1, CurveID: 1, PublicKey: pub,
		Extensions: cert.Extensions{
			BasicConstraints: &cert.BasicConstraints{IsCA: false},
			KeyUsage:         u16p(0x01),
			ExtKeyUsage:      []uint8{2, 1},
			SubjectKeyID:     bytes.Repeat([]byte{0xBB}, 20),
			AuthorityKeyID:   bytes.Repeat([]byte{0xAA}, 20),
		},
	}
}

func buildFabric(t *testing.T) (casesession.Fabric, casesession.Identity, casesession.Identity) {
	rootKey, rootPub := genKey(t)
	rcac := &cert.Cert{
		SerialNumber: []byte{0x01}, SigAlgo: 1,
		Issuer:    cert.DN{Attrs: []cert.Attr{{Tag: cert.DNMatterRCACID, Value: rootID}}},
		NotBefore: 0x271b17ef, NotAfter: 0x4cb9b56e,
		Subject:   cert.DN{Attrs: []cert.Attr{{Tag: cert.DNMatterRCACID, Value: rootID}}},
		PubKeyAlgo: 1, CurveID: 1, PublicKey: rootPub,
		Extensions: cert.Extensions{
			BasicConstraints: &cert.BasicConstraints{IsCA: true, PathLen: u8p(1)},
			KeyUsage:         u16p(0x60),
			SubjectKeyID:     bytes.Repeat([]byte{0xAA}, 20),
			AuthorityKeyID:   bytes.Repeat([]byte{0xAA}, 20),
		},
	}
	rcacTLV, err := rcac.SignAndEncode(rootKey)
	if err != nil {
		t.Fatal(err)
	}
	rcacDec, err := cert.Decode(rcacTLV)
	if err != nil {
		t.Fatal(err)
	}

	ctrlKey, ctrlPub := genKey(t)
	devKey, devPub := genKey(t)
	ctrlNOC, err := noc(ctrlPub, ctrlNode).SignAndEncode(rootKey)
	if err != nil {
		t.Fatal(err)
	}
	devNOC, err := noc(devPub, devNode).SignAndEncode(rootKey)
	if err != nil {
		t.Fatal(err)
	}

	fabric := casesession.Fabric{IPK: bytes.Repeat([]byte{0x77}, 16), FabricID: fabricID, RootPubKey: rootPub, RCAC: rcacDec}
	return fabric, casesession.Identity{NOC: ctrlNOC, OpKey: ctrlKey}, casesession.Identity{NOC: devNOC, OpKey: devKey}
}

// runDevice acts as a Matter device over t: it completes the CASE handshake and
// answers Invoke (success) and Read (lift position 37%) requests.
func runDevice(ctx context.Context, t *testing.T, tp transport.Transport, fabric casesession.Fabric, self casesession.Identity) {
	responder, err := casesession.NewResponder(fabric, self, 0x2002)
	if err != nil {
		t.Errorf("device responder: %v", err)
		return
	}

	f1, err := tp.Receive(ctx)
	if err != nil {
		return
	}
	p1, sigma1, err := parseUnsecured(f1)
	if err != nil {
		t.Errorf("device sigma1: %v", err)
		return
	}
	sigma2, err := responder.HandleSigma1(sigma1)
	if err != nil {
		t.Errorf("device handle sigma1: %v", err)
		return
	}
	tp.Send(frameUnsecured(0, p1.ExchangeID, false, message.SCCASESigma2, sigma2))

	f3, err := tp.Receive(ctx)
	if err != nil {
		return
	}
	p3, sigma3, err := parseUnsecured(f3)
	if err != nil {
		t.Errorf("device sigma3: %v", err)
		return
	}
	if err := responder.HandleSigma3(sigma3); err != nil {
		t.Errorf("device handle sigma3: %v", err)
		return
	}
	sr := message.StatusReport{GeneralCode: message.GeneralSuccess, ProtocolID: uint32(message.ProtocolSecureChannel)}
	tp.Send(frameUnsecured(1, p3.ExchangeID, false, message.SCStatusReport, sr.Encode()))

	secure, err := responder.SecureSession()
	if err != nil {
		t.Errorf("device session: %v", err)
		return
	}

	for {
		f, err := tp.Receive(ctx)
		if err != nil {
			return
		}
		payload, err := secure.Decrypt(f)
		if err != nil {
			t.Errorf("device decrypt: %v", err)
			return
		}
		ph, imBytes, err := message.DecodeProto(payload)
		if err != nil {
			t.Errorf("device proto: %v", err)
			return
		}
		switch ph.Opcode {
		case message.IMInvokeRequest:
			cmds, _, _, _ := im.DecodeInvokeRequest(imBytes)
			results := []im.InvokeResult{{Status: &im.CommandStatus{Path: cmds[0].Path, Status: im.StatusSuccess}}}
			resp, _ := im.EncodeInvokeResponse(results, false)
			rp := message.ProtoHeader{Opcode: message.IMInvokeResponse, ExchangeID: ph.ExchangeID, ProtocolID: message.ProtocolInteractionModel}
			frame, _ := secure.Encrypt(append(rp.Encode(), resp...))
			tp.Send(frame)
		case message.IMReadRequest:
			paths, _, _ := im.DecodeReadRequest(imBytes)
			vw := tlv.NewWriter()
			vw.PutUint(tlv.Anonymous(), 3700)
			val, _ := vw.Bytes()
			reports := []im.AttributeReport{{Path: paths[0], DataVersion: 1, Data: val}}
			rd, _ := im.EncodeReportData(0, reports, false)
			rp := message.ProtoHeader{Opcode: message.IMReportData, ExchangeID: ph.ExchangeID, ProtocolID: message.ProtocolInteractionModel}
			frame, _ := secure.Encrypt(append(rp.Encode(), rd...))
			tp.Send(frame)
		}
	}
}

func TestControllerConnectInvokeRead(t *testing.T) {
	fabric, ctrlID, devID := buildFabric(t)

	ctrlPipe, devPipe := transport.NewPipe()
	defer ctrlPipe.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go runDevice(ctx, t, devPipe, fabric, devID)

	ctrl := New(fabric, ctrlID)
	sess, err := ctrl.Connect(ctx, ctrlPipe, devNode)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	// Invoke GoToLiftPercentage(37%).
	cmd, err := cluster.GoToLiftPercentage(1, 37)
	if err != nil {
		t.Fatal(err)
	}
	res, err := sess.Invoke(ctx, cmd)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if res.Status == nil || res.Status.Status != im.StatusSuccess {
		t.Fatalf("invoke result: %+v", res)
	}

	// Read the current lift position.
	rep, err := sess.ReadAttribute(ctx, cluster.LiftPositionAttribute(1))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	pct, err := cluster.DecodeLiftPercent(rep.Data)
	if err != nil || pct != 37 {
		t.Fatalf("lift percent = %g (%v)", pct, err)
	}
}
