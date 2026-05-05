package matter

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
	"github.com/sh5080/go-matter/controller"
	"github.com/sh5080/go-matter/crypto"
	"github.com/sh5080/go-matter/im"
	"github.com/sh5080/go-matter/message"
	"github.com/sh5080/go-matter/tlv"
	"github.com/sh5080/go-matter/transport"
)

// This test stands up an in-memory Matter window-covering device and drives it
// through GoMatterDriver, validating the full hub→go-matter control path.

const (
	tFabricID = 0xFAB000000000001D
	tRootID   = 0xCACACACA00000001
	tCtrlNode = 0x1111000000000001
	tDevNode  = 0x2222000000000002
	wcCluster = 0x0102
	wcGoTo    = 0x05
	wcUpOpen  = 0x00
	wcDown    = 0x01
	wcLiftPos = 0x000E
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

func nocCert(pub []byte, node uint64) *cert.Cert {
	return &cert.Cert{
		SerialNumber: []byte{0x02}, SigAlgo: 1,
		Issuer:    cert.DN{Attrs: []cert.Attr{{Tag: cert.DNMatterRCACID, Value: tRootID}}},
		NotBefore: 0x271b17ef, NotAfter: 0x4cb9b56e,
		Subject: cert.DN{Attrs: []cert.Attr{
			{Tag: cert.DNMatterNodeID, Value: node},
			{Tag: cert.DNMatterFabricID, Value: tFabricID},
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
		Issuer:    cert.DN{Attrs: []cert.Attr{{Tag: cert.DNMatterRCACID, Value: tRootID}}},
		NotBefore: 0x271b17ef, NotAfter: 0x4cb9b56e,
		Subject:   cert.DN{Attrs: []cert.Attr{{Tag: cert.DNMatterRCACID, Value: tRootID}}},
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
	ctrlNOC, err := nocCert(ctrlPub, tCtrlNode).SignAndEncode(rootKey)
	if err != nil {
		t.Fatal(err)
	}
	devNOC, err := nocCert(devPub, tDevNode).SignAndEncode(rootKey)
	if err != nil {
		t.Fatal(err)
	}
	fabric := casesession.Fabric{IPK: bytes.Repeat([]byte{0x77}, 16), FabricID: tFabricID, RootPubKey: rootPub, RCAC: rcacDec}
	return fabric, casesession.Identity{NOC: ctrlNOC, OpKey: ctrlKey}, casesession.Identity{NOC: devNOC, OpKey: devKey}
}

// frameUnsecured / parseUnsecured mirror the controller's unsecured framing so
// the test device speaks the same wire format for the CASE handshake.
func frameUnsecured(counter uint32, exch uint16, initiator bool, opcode byte, payload []byte) []byte {
	hdr := message.Header{SessionType: message.Unicast, Counter: counter}
	aad, _ := hdr.Encode()
	p := message.ProtoHeader{Initiator: initiator, Opcode: opcode, ExchangeID: exch, ProtocolID: message.ProtocolSecureChannel}
	return append(append(aad, p.Encode()...), payload...)
}

func parseUnsecured(frame []byte) (message.ProtoHeader, []byte, error) {
	hdr, rest, err := message.Decode(frame)
	if err != nil {
		return message.ProtoHeader{}, nil, err
	}
	_ = hdr
	return message.DecodeProto(rest)
}

// runWindowCovering is a stateful device that answers WindowCovering commands
// and lift-position reads over t.
func runWindowCovering(ctx context.Context, t *testing.T, tp transport.Transport, fabric casesession.Fabric, self casesession.Identity) {
	responder, err := casesession.NewResponder(fabric, self, 0x2002)
	if err != nil {
		t.Errorf("responder: %v", err)
		return
	}
	f1, err := tp.Receive(ctx)
	if err != nil {
		return
	}
	p1, sig1, err := parseUnsecured(f1)
	if err != nil {
		t.Errorf("sigma1: %v", err)
		return
	}
	sig2, err := responder.HandleSigma1(sig1)
	if err != nil {
		t.Errorf("handle sigma1: %v", err)
		return
	}
	tp.Send(frameUnsecured(0, p1.ExchangeID, false, message.SCCASESigma2, sig2))

	f3, err := tp.Receive(ctx)
	if err != nil {
		return
	}
	p3, sig3, err := parseUnsecured(f3)
	if err != nil {
		t.Errorf("sigma3: %v", err)
		return
	}
	if err := responder.HandleSigma3(sig3); err != nil {
		t.Errorf("handle sigma3: %v", err)
		return
	}
	sr := message.StatusReport{GeneralCode: message.GeneralSuccess, ProtocolID: uint32(message.ProtocolSecureChannel)}
	tp.Send(frameUnsecured(1, p3.ExchangeID, false, message.SCStatusReport, sr.Encode()))

	secure, err := responder.SecureSession()
	if err != nil {
		t.Errorf("session: %v", err)
		return
	}

	var lift100ths uint64 // current lift position, hundredths of a percent
	for {
		f, err := tp.Receive(ctx)
		if err != nil {
			return
		}
		payload, err := secure.Decrypt(f)
		if err != nil {
			return
		}
		ph, imb, err := message.DecodeProto(payload)
		if err != nil {
			return
		}
		switch ph.Opcode {
		case message.IMInvokeRequest:
			cmds, _, _, _ := im.DecodeInvokeRequest(imb)
			c := cmds[0]
			switch c.Path.Command {
			case wcGoTo:
				lift100ths, _ = im.DecodeUint(c.Fields)
			case wcUpOpen:
				lift100ths = 0
			case wcDown:
				lift100ths = 10000
			}
			res := []im.InvokeResult{{Status: &im.CommandStatus{Path: c.Path, Status: im.StatusSuccess}}}
			resp, _ := im.EncodeInvokeResponse(res, false)
			rp := message.ProtoHeader{Opcode: message.IMInvokeResponse, ExchangeID: ph.ExchangeID, ProtocolID: message.ProtocolInteractionModel}
			frame, _ := secure.Encrypt(append(rp.Encode(), resp...))
			tp.Send(frame)
		case message.IMReadRequest:
			paths, _, _ := im.DecodeReadRequest(imb)
			vw := tlv.NewWriter()
			vw.PutUint(tlv.Anonymous(), lift100ths)
			val, _ := vw.Bytes()
			reports := []im.AttributeReport{{Path: paths[0], DataVersion: 1, Data: val}}
			rd, _ := im.EncodeReportData(0, reports, false)
			rp := message.ProtoHeader{Opcode: message.IMReportData, ExchangeID: ph.ExchangeID, ProtocolID: message.ProtocolInteractionModel}
			frame, _ := secure.Encrypt(append(rp.Encode(), rd...))
			tp.Send(frame)
		}
	}
}

func TestGoMatterDriverControlsDevice(t *testing.T) {
	fabric, ctrlID, devID := buildFabric(t)

	ctrlPipe, devPipe := transport.NewPipe()
	defer ctrlPipe.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go runWindowCovering(ctx, t, devPipe, fabric, devID)

	sess, err := controller.New(fabric, ctrlID).Connect(ctx, ctrlPipe, tDevNode)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	drv := NewGoMatterDriver(sess, 1)

	if err := drv.SetLiftPercent(37); err != nil {
		t.Fatalf("SetLiftPercent: %v", err)
	}
	pct, err := drv.LiftPercent()
	if err != nil || pct != 37 {
		t.Fatalf("LiftPercent = %d (%v), want 37", pct, err)
	}

	if err := drv.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if pct, _ := drv.LiftPercent(); pct != 0 {
		t.Fatalf("after Open, LiftPercent = %d, want 0", pct)
	}

	if err := drv.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if pct, _ := drv.LiftPercent(); pct != 100 {
		t.Fatalf("after Close, LiftPercent = %d, want 100", pct)
	}
}
