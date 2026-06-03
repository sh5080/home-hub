package controller

import (
	"context"
	"testing"
	"time"

	"github.com/sh5080/go-matter/casesession"
	"github.com/sh5080/go-matter/cluster"
	"github.com/sh5080/go-matter/im"
	"github.com/sh5080/go-matter/message"
	"github.com/sh5080/go-matter/session"
	"github.com/sh5080/go-matter/tlv"
	"github.com/sh5080/go-matter/transport"
)

// caseAccept completes the responder side of CASE and returns the secure session.
func caseAccept(ctx context.Context, t *testing.T, tp transport.Transport, fabric casesession.Fabric, self casesession.Identity) *session.Secure {
	t.Helper()
	responder, err := casesession.NewResponder(fabric, self, 0x2002)
	if err != nil {
		t.Errorf("responder: %v", err)
		return nil
	}
	f1, err := tp.Receive(ctx)
	if err != nil {
		return nil
	}
	p1, sigma1, err := parseUnsecured(f1)
	if err != nil {
		t.Errorf("sigma1: %v", err)
		return nil
	}
	sigma2, err := responder.HandleSigma1(sigma1)
	if err != nil {
		t.Errorf("handle sigma1: %v", err)
		return nil
	}
	tp.Send(frameUnsecured(0, p1.ExchangeID, false, message.SCCASESigma2, sigma2))

	f3, err := tp.Receive(ctx)
	if err != nil {
		return nil
	}
	p3, sigma3, err := parseUnsecured(f3)
	if err != nil {
		t.Errorf("sigma3: %v", err)
		return nil
	}
	if err := responder.HandleSigma3(sigma3); err != nil {
		t.Errorf("handle sigma3: %v", err)
		return nil
	}
	sr := message.StatusReport{GeneralCode: message.GeneralSuccess, ProtocolID: uint32(message.ProtocolSecureChannel)}
	tp.Send(frameUnsecured(1, p3.ExchangeID, false, message.SCStatusReport, sr.Encode()))

	secure, err := responder.SecureSession()
	if err != nil {
		t.Errorf("secure session: %v", err)
		return nil
	}
	return secure
}

// sendReport encrypts and sends a ReportData for one lift-percent value on the
// given exchange, as a device would.
func sendReport(t *testing.T, tp transport.Transport, secure *session.Secure, exchange uint16, subID uint32, value uint64) {
	t.Helper()
	vw := tlv.NewWriter()
	vw.PutUint(tlv.Anonymous(), value)
	val, _ := vw.Bytes()
	reports := []im.AttributeReport{{Path: cluster.LiftPositionAttribute(1), DataVersion: 1, Data: val}}
	rd, _ := im.EncodeReportData(subID, reports, false)
	rp := message.ProtoHeader{Opcode: message.IMReportData, ExchangeID: exchange, ProtocolID: message.ProtocolInteractionModel}
	frame, _ := secure.Encrypt(append(rp.Encode(), rd...))
	tp.Send(frame)
}

// runSubscribeDevice completes CASE, answers a Subscribe (priming report + ack +
// SubscribeResponse), then pushes `extra` further reports, acking-in-reverse.
func runSubscribeDevice(ctx context.Context, t *testing.T, tp transport.Transport, fabric casesession.Fabric, self casesession.Identity, primeVal uint64, extra []uint64) {
	secure := caseAccept(ctx, t, tp, fabric, self)
	if secure == nil {
		return
	}
	recv := func() (message.ProtoHeader, []byte) {
		f, err := tp.Receive(ctx)
		if err != nil {
			return message.ProtoHeader{}, nil
		}
		payload, err := secure.Decrypt(f)
		if err != nil {
			t.Errorf("device decrypt: %v", err)
			return message.ProtoHeader{}, nil
		}
		ph, imb, err := message.DecodeProto(payload)
		if err != nil {
			t.Errorf("device proto: %v", err)
		}
		return ph, imb
	}

	// Subscribe setup.
	ph, _ := recv()
	if ph.Opcode != message.IMSubscribeRequest {
		t.Errorf("device: expected SubscribeRequest, got 0x%02x", ph.Opcode)
		return
	}
	subEx := ph.ExchangeID
	sendReport(t, tp, secure, subEx, 1, primeVal) // priming report

	if ph, _ = recv(); ph.Opcode != message.IMStatusResponse {
		t.Errorf("device: expected StatusResponse ack, got 0x%02x", ph.Opcode)
		return
	}
	resp, _ := im.EncodeSubscribeResponse(1, 5)
	rp := message.ProtoHeader{Opcode: message.IMSubscribeResponse, ExchangeID: subEx, ProtocolID: message.ProtocolInteractionModel}
	frame, _ := secure.Encrypt(append(rp.Encode(), resp...))
	tp.Send(frame)

	// Subsequent device-initiated reports.
	for i, v := range extra {
		ex := uint16(0x8000 + i)
		sendReport(t, tp, secure, ex, 1, v)
		if ph, _ = recv(); ph.Opcode != message.IMStatusResponse {
			t.Errorf("device: expected ack for report %d, got 0x%02x", i, ph.Opcode)
			return
		}
	}
}

func TestSubscribeSetup(t *testing.T) {
	fabric, ctrlID, devID := buildFabric(t)
	ctrlPipe, devPipe := transport.NewPipe()
	defer ctrlPipe.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go runSubscribeDevice(ctx, t, devPipe, fabric, devID, 3700, nil)

	sess, err := New(fabric, ctrlID).Connect(ctx, ctrlPipe, devNode)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	sub, err := sess.Subscribe(ctx, []im.AttributePath{cluster.LiftPositionAttribute(1)}, 0, 10)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if sub.ID != 1 || sub.MaxInterval != 5 {
		t.Fatalf("subscription = %+v", sub)
	}
	if len(sub.Initial) != 1 {
		t.Fatalf("initial reports = %d, want 1", len(sub.Initial))
	}
	pct, err := cluster.DecodeLiftPercent(sub.Initial[0].Data)
	if err != nil || pct != 37 {
		t.Fatalf("priming lift = %g (%v)", pct, err)
	}
}
