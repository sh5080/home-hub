package controller

import (
	"bytes"
	"context"
	"errors"
	"sync"
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

// floodTransport hands back an endless stream of authenticating standalone-ack
// frames, simulating a peer that keeps the socket fed so the read never times
// out. exchangeRT must still terminate (bounded) instead of looping forever.
type floodTransport struct {
	peer   *session.Secure // encrypts frames the session-under-test will accept
	closed bool
}

func (f *floodTransport) Send([]byte) error { return nil }
func (f *floodTransport) Close() error      { f.closed = true; return nil }
func (f *floodTransport) Receive(ctx context.Context) ([]byte, error) {
	if f.closed || ctx.Err() != nil {
		return nil, context.Canceled
	}
	ack := message.ProtoHeader{
		Opcode: message.SCStandaloneAck, ExchangeID: 0x9999,
		ProtocolID: message.ProtocolSecureChannel,
	}
	frame, err := f.peer.Encrypt(ack.Encode())
	if err != nil {
		return nil, err
	}
	return frame, nil
}

// TestExchangeRTBoundedUnderFlood proves the anti-flood cap: a peer that never
// stops sending non-reply frames makes exchangeRT return an error rather than
// spin forever or reflect unboundedly.
func TestExchangeRTBoundedUnderFlood(t *testing.T) {
	sendKey := bytes.Repeat([]byte{0x11}, 16)
	recvKey := bytes.Repeat([]byte{0x22}, 16)
	// The session under test (local id 1) receives with recvKey; the peer sends
	// with the same key so its frames authenticate.
	ours, err := session.NewSecure(1, 2, 0, 0, sendKey, recvKey, 0)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := session.NewSecure(2, 1, 0, 0, recvKey, sendKey, 0)
	if err != nil {
		t.Fatal(err)
	}

	sess := &Session{secure: ours, t: &floodTransport{peer: peer}, exchange: 1}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, e := sess.exchangeRT(ctx, []byte("sent"), 1)
		done <- e
	}()
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("expected an error under an endless flood")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("exchangeRT did not terminate under flood (unbounded loop)")
	}
}

func TestMRPInterval(t *testing.T) {
	if mrpInterval(0) != mrpInitialInterval {
		t.Fatalf("interval(0) = %v", mrpInterval(0))
	}
	if mrpInterval(1) != 2*mrpInitialInterval {
		t.Fatalf("interval(1) = %v", mrpInterval(1))
	}
	if mrpInterval(10) != mrpIntervalCeiling {
		t.Fatalf("interval(10) = %v, want ceiling", mrpInterval(10))
	}
}

// lossyTransport drops the first transmission of every distinct frame and
// passes retransmissions through — the worst deterministic single-loss network.
type lossyTransport struct {
	inner   transport.Transport
	mu      sync.Mutex
	seen    map[string]bool
	dropped int
}

func newLossy(inner transport.Transport) *lossyTransport {
	return &lossyTransport{inner: inner, seen: make(map[string]bool)}
}

func (l *lossyTransport) Send(frame []byte) error {
	l.mu.Lock()
	first := !l.seen[string(frame)]
	if first {
		l.seen[string(frame)] = true
		l.dropped++
	}
	l.mu.Unlock()
	if first {
		return nil // dropped on the floor
	}
	return l.inner.Send(frame)
}

func (l *lossyTransport) Receive(ctx context.Context) ([]byte, error) { return l.inner.Receive(ctx) }
func (l *lossyTransport) Close() error                                { return l.inner.Close() }

func (l *lossyTransport) drops() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.dropped
}

// runMRPDevice is a device that itself follows minimal MRP: it deduplicates by
// message counter and answers a retransmission by re-sending its last frame.
// It completes CASE, then answers Invoke (success) and Read (lift 37%).
func runMRPDevice(ctx context.Context, t *testing.T, tp transport.Transport, fabric casesession.Fabric, self casesession.Identity) {
	responder, err := casesession.NewResponder(fabric, self, 0x2002)
	if err != nil {
		t.Errorf("device responder: %v", err)
		return
	}

	seen := make(map[uint32]bool)
	var last []byte // our last reliable frame, re-sent when the peer retransmits
	var devCounter uint32
	send := func(f []byte) {
		last = f
		tp.Send(f)
	}

	// CASE phase (unsecured messages).
	var secure *session.Secure
	for secure == nil {
		f, err := tp.Receive(ctx)
		if err != nil {
			return
		}
		hdr, ph, payload, perr := parseUnsecuredMsg(f)
		if perr != nil {
			continue
		}
		if isStandaloneAck(ph) {
			seen[hdr.Counter] = true
			continue
		}
		if seen[hdr.Counter] {
			if last != nil {
				tp.Send(last)
			}
			continue
		}
		seen[hdr.Counter] = true
		switch ph.Opcode {
		case message.SCCASESigma1:
			sigma2, err := responder.HandleSigma1(payload)
			if err != nil {
				t.Errorf("device sigma1: %v", err)
				return
			}
			send(frameUnsecured(devCounter, ph.ExchangeID, false, message.SCCASESigma2, sigma2))
			devCounter++
		case message.SCCASESigma3:
			if err := responder.HandleSigma3(payload); err != nil {
				t.Errorf("device sigma3: %v", err)
				return
			}
			sr := message.StatusReport{GeneralCode: message.GeneralSuccess, ProtocolID: uint32(message.ProtocolSecureChannel)}
			send(frameUnsecuredAck(devCounter, ph.ExchangeID, false, message.SCStatusReport, sr.Encode(), hdr.Counter, true))
			devCounter++
			if secure, err = responder.SecureSession(); err != nil {
				t.Errorf("device session: %v", err)
				return
			}
		}
	}

	// Operational phase (secure messages).
	var lastSec []byte
	for {
		f, err := tp.Receive(ctx)
		if err != nil {
			return
		}
		if isUnsecuredFrame(f) {
			// A retransmitted CASE message (e.g. Sigma3 whose StatusReport was
			// lost): re-send our final handshake frame so the peer completes.
			if hdr, ph, _, perr := parseUnsecuredMsg(f); perr == nil && !isStandaloneAck(ph) && seen[hdr.Counter] && last != nil {
				tp.Send(last)
			}
			continue
		}
		counter, payload, derr := secure.DecryptMsg(f)
		if errors.Is(derr, session.ErrReplay) {
			if lastSec != nil {
				tp.Send(lastSec)
			}
			continue
		}
		if derr != nil {
			t.Errorf("device decrypt: %v", derr)
			return
		}
		ph, imBytes, err := message.DecodeProto(payload)
		if err != nil {
			t.Errorf("device proto: %v", err)
			return
		}
		if isStandaloneAck(ph) {
			continue
		}
		reply := func(opcode byte, body []byte) {
			rp := message.ProtoHeader{
				Initiator: false, Reliable: true, Opcode: opcode,
				ExchangeID: ph.ExchangeID, ProtocolID: message.ProtocolInteractionModel,
				AckPresent: true, AckCounter: counter,
			}
			frame, _ := secure.Encrypt(append(rp.Encode(), body...))
			lastSec = frame
			tp.Send(frame)
		}
		switch ph.Opcode {
		case message.IMInvokeRequest:
			cmds, _, _, _ := im.DecodeInvokeRequest(imBytes)
			results := []im.InvokeResult{{Status: &im.CommandStatus{Path: cmds[0].Path, Status: im.StatusSuccess}}}
			resp, _ := im.EncodeInvokeResponse(results, false)
			reply(message.IMInvokeResponse, resp)
		case message.IMReadRequest:
			paths, _, _ := im.DecodeReadRequest(imBytes)
			vw := tlv.NewWriter()
			vw.PutUint(tlv.Anonymous(), 3700)
			val, _ := vw.Bytes()
			reports := []im.AttributeReport{{Path: paths[0], DataVersion: 1, Data: val}}
			rd, _ := im.EncodeReportData(0, reports, false)
			reply(message.IMReportData, rd)
		}
	}
}

// TestMRPSurvivesPacketLoss proves CASE + Invoke + Read complete even when the
// first transmission of every single frame — in both directions — is lost.
func TestMRPSurvivesPacketLoss(t *testing.T) {
	fabric, ctrlID, devID := buildFabric(t)
	ctrlPipe, devPipe := transport.NewPipe()
	defer ctrlPipe.Close()

	ctrlSide := newLossy(ctrlPipe)
	devSide := newLossy(devPipe)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	go runMRPDevice(ctx, t, devSide, fabric, devID)

	sess, err := New(fabric, ctrlID).Connect(ctx, ctrlSide, devNode)
	if err != nil {
		t.Fatalf("connect under loss: %v", err)
	}

	cmd, err := cluster.GoToLiftPercentage(1, 37)
	if err != nil {
		t.Fatal(err)
	}
	res, err := sess.Invoke(ctx, cmd)
	if err != nil {
		t.Fatalf("invoke under loss: %v", err)
	}
	if res.Status == nil || res.Status.Status != im.StatusSuccess {
		t.Fatalf("invoke result: %+v", res)
	}

	rep, err := sess.ReadAttribute(ctx, cluster.LiftPositionAttribute(1))
	if err != nil {
		t.Fatalf("read under loss: %v", err)
	}
	pct, err := cluster.DecodeLiftPercent(rep.Data)
	if err != nil || pct != 37 {
		t.Fatalf("lift percent = %g (%v)", pct, err)
	}

	if ctrlSide.drops() == 0 || devSide.drops() == 0 {
		t.Fatalf("loss was not exercised: ctrl=%d dev=%d drops", ctrlSide.drops(), devSide.drops())
	}
}
