package controller

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/sh5080/go-matter/casesession"
	"github.com/sh5080/go-matter/im"
	"github.com/sh5080/go-matter/message"
	"github.com/sh5080/go-matter/session"
	"github.com/sh5080/go-matter/transport"
)

// Controller is an operational Matter controller bound to one fabric and
// operational identity. It establishes CASE sessions with devices and exposes
// the Interaction Model over them.
type Controller struct {
	fabric casesession.Fabric
	self   casesession.Identity

	mu           sync.Mutex // guards the session/exchange id counters
	nextSession  uint16
	nextExchange uint16
}

// New creates a controller for the given fabric and identity.
func New(fabric casesession.Fabric, self casesession.Identity) *Controller {
	return &Controller{fabric: fabric, self: self, nextSession: 1, nextExchange: 1}
}

// nextIDs hands out a fresh (session, exchange) id pair, safe for concurrent
// Connect/Dial/Commission calls.
func (c *Controller) nextIDs() (session, exchange uint16) {
	c.mu.Lock()
	defer c.mu.Unlock()
	session, exchange = c.nextSession, c.nextExchange
	c.nextSession++
	c.nextExchange++
	return
}

// Connect performs a CASE handshake with peerNodeID over t and returns the
// resulting secure Session.
func (c *Controller) Connect(ctx context.Context, t transport.Transport, peerNodeID uint64) (*Session, error) {
	localSessionID, exchangeID := c.nextIDs()

	in, err := casesession.NewInitiator(c.fabric, c.self, peerNodeID, localSessionID)
	if err != nil {
		return nil, err
	}

	s1, err := in.Sigma1()
	if err != nil {
		return nil, err
	}
	seen := make(map[uint32]bool) // peer message counters already processed

	sigma1Frame := frameUnsecured(0, exchangeID, true, message.SCCASESigma1, s1)
	if err := t.Send(sigma1Frame); err != nil {
		return nil, err
	}
	hdr2, proto2, sigma2, err := awaitSC(ctx, t, sigma1Frame, seen)
	if err != nil {
		return nil, err
	}
	if proto2.Opcode == message.SCStatusReport {
		return nil, statusReportError(sigma2)
	}
	if proto2.Opcode != message.SCCASESigma2 {
		return nil, fmt.Errorf("controller: expected Sigma2, got opcode 0x%02x", proto2.Opcode)
	}

	s3, err := in.HandleSigma2(sigma2)
	if err != nil {
		return nil, err
	}
	// Sigma3 piggybacks the MRP acknowledgement of Sigma2.
	sigma3Frame := frameUnsecuredAck(1, exchangeID, true, message.SCCASESigma3, s3, hdr2.Counter, true)
	if err := t.Send(sigma3Frame); err != nil {
		return nil, err
	}

	// The responder confirms the session with a SUCCESS StatusReport. A
	// retransmitted Sigma2 (our Sigma3 was lost) is answered by awaitSC with the
	// Sigma3 frame again.
	hdr4, proto4, sr, err := awaitSC(ctx, t, sigma3Frame, seen)
	if err != nil {
		return nil, err
	}
	if proto4.Opcode != message.SCStatusReport {
		return nil, fmt.Errorf("controller: expected StatusReport, got opcode 0x%02x", proto4.Opcode)
	}
	if err := statusReportError(sr); err != nil {
		return nil, err
	}
	// Standalone-ack the StatusReport so the responder can retire the exchange.
	_ = t.Send(frameUnsecuredAck(2, exchangeID, true, message.SCStandaloneAck, nil, hdr4.Counter, true))

	secure, err := in.SecureSession()
	if err != nil {
		return nil, err
	}
	return &Session{secure: secure, t: t, exchange: exchangeID + 1}, nil
}

// awaitSC waits for a fresh (non-duplicate) Secure Channel message during the
// CASE handshake. It retransmits `resend` — our last reliable frame — both on
// timeout and when the peer retransmits a message we already processed, and it
// skips standalone acks.
func awaitSC(ctx context.Context, t transport.Transport, resend []byte, seen map[uint32]bool) (message.Header, message.ProtoHeader, []byte, error) {
	retransmits := 0
	for iters := 0; ; iters++ {
		// Bound the loop and honor ctx so an injected flood of unparseable or
		// already-seen unsecured frames during the handshake cannot spin here
		// forever (or grow `seen` without limit).
		if err := ctx.Err(); err != nil {
			return message.Header{}, message.ProtoHeader{}, nil, err
		}
		if iters > maxExchangeMessages {
			return message.Header{}, message.ProtoHeader{}, nil,
				errors.New("controller: too many handshake messages without progress")
		}
		actx, cancel := context.WithTimeout(ctx, mrpInterval(retransmits))
		frame, err := t.Receive(actx)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return message.Header{}, message.ProtoHeader{}, nil, ctx.Err()
			}
			if errors.Is(err, context.DeadlineExceeded) {
				if retransmits >= mrpMaxRetransmits {
					return message.Header{}, message.ProtoHeader{}, nil,
						errors.New("controller: handshake timed out after MRP retransmissions")
				}
				if serr := t.Send(resend); serr != nil {
					return message.Header{}, message.ProtoHeader{}, nil, serr
				}
				retransmits++
				continue
			}
			return message.Header{}, message.ProtoHeader{}, nil, err
		}
		hdr, ph, payload, perr := parseUnsecuredMsg(frame)
		if perr != nil {
			continue // not an unsecured SC message; ignore during handshake
		}
		if isStandaloneAck(ph) {
			seen[hdr.Counter] = true
			continue
		}
		if seen[hdr.Counter] {
			// Peer retransmission: it missed our last frame (or its ack) — resend.
			if serr := t.Send(resend); serr != nil {
				return message.Header{}, message.ProtoHeader{}, nil, serr
			}
			continue
		}
		seen[hdr.Counter] = true
		return hdr, ph, payload, nil
	}
}

func statusReportError(payload []byte) error {
	sr, err := message.DecodeStatusReport(payload)
	if err != nil {
		return fmt.Errorf("controller: malformed status report: %w", err)
	}
	if e := sr.Error(); e != nil {
		return fmt.Errorf("controller: handshake rejected: %w", e)
	}
	return nil
}

// Session is an established operational session with a device.
//
// A Session is safe for concurrent use: every request/response round trip is
// serialized by an internal mutex, so a command dispatcher and a state poller
// can share it without interleaving frames or racing the counters. The
// exception is an active Subscription, whose Listen owns the receive path for
// its whole lifetime — dedicate a session to it (see Subscription docs).
type Session struct {
	secure   *session.Secure
	t        transport.Transport
	mu       sync.Mutex // serializes exchanges; protects exchange counter
	exchange uint16
}

func (s *Session) nextExchange() uint16 {
	e := s.exchange
	s.exchange++
	return e
}

// roundTrip sends an IM request and returns the response's protocol header and
// IM payload, holding the session mutex for the whole exchange. Reliability is
// handled by exchangeRT; the response itself is standalone-acked so the device
// can retire it.
func (s *Session) roundTrip(ctx context.Context, opcode byte, imPayload []byte) (message.ProtoHeader, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	exchangeID := s.nextExchange()
	proto := message.ProtoHeader{
		Initiator: true, Reliable: true, Opcode: opcode,
		ExchangeID: exchangeID, ProtocolID: message.ProtocolInteractionModel,
	}
	frame, err := s.encryptProto(proto, imPayload)
	if err != nil {
		return message.ProtoHeader{}, nil, err
	}
	if err := s.t.Send(frame); err != nil {
		return message.ProtoHeader{}, nil, err
	}
	m, err := s.exchangeRT(ctx, frame, exchangeID)
	if err != nil {
		return message.ProtoHeader{}, nil, err
	}
	if err := s.ackMsg(m); err != nil {
		return message.ProtoHeader{}, nil, err
	}
	return m.ph, m.payload, nil
}

// Invoke sends a single command and returns its result.
func (s *Session) Invoke(ctx context.Context, cmd im.InvokeCommand) (im.InvokeResult, error) {
	req, err := im.EncodeInvokeRequest([]im.InvokeCommand{cmd}, false, false)
	if err != nil {
		return im.InvokeResult{}, err
	}
	ph, respIM, err := s.roundTrip(ctx, message.IMInvokeRequest, req)
	if err != nil {
		return im.InvokeResult{}, err
	}
	if ph.Opcode != message.IMInvokeResponse {
		return im.InvokeResult{}, fmt.Errorf("controller: expected InvokeResponse, got 0x%02x", ph.Opcode)
	}
	results, err := im.DecodeInvokeResponse(respIM)
	if err != nil {
		return im.InvokeResult{}, err
	}
	if len(results) != 1 {
		return im.InvokeResult{}, fmt.Errorf("controller: expected 1 invoke result, got %d", len(results))
	}
	return results[0], nil
}

// ReadAttribute reads a single attribute and returns its report.
func (s *Session) ReadAttribute(ctx context.Context, path im.AttributePath) (im.AttributeReport, error) {
	req, err := im.EncodeReadRequest([]im.AttributePath{path}, true)
	if err != nil {
		return im.AttributeReport{}, err
	}
	ph, respIM, err := s.roundTrip(ctx, message.IMReadRequest, req)
	if err != nil {
		return im.AttributeReport{}, err
	}
	if ph.Opcode != message.IMReportData {
		return im.AttributeReport{}, fmt.Errorf("controller: expected ReportData, got 0x%02x", ph.Opcode)
	}
	_, reports, err := im.DecodeReportData(respIM)
	if err != nil {
		return im.AttributeReport{}, err
	}
	if len(reports) != 1 {
		return im.AttributeReport{}, fmt.Errorf("controller: expected 1 report, got %d", len(reports))
	}
	return reports[0], nil
}
