package controller

import (
	"context"
	"fmt"

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
	fabric       casesession.Fabric
	self         casesession.Identity
	nextSession  uint16
	nextExchange uint16
}

// New creates a controller for the given fabric and identity.
func New(fabric casesession.Fabric, self casesession.Identity) *Controller {
	return &Controller{fabric: fabric, self: self, nextSession: 1, nextExchange: 1}
}

// Connect performs a CASE handshake with peerNodeID over t and returns the
// resulting secure Session.
func (c *Controller) Connect(ctx context.Context, t transport.Transport, peerNodeID uint64) (*Session, error) {
	localSessionID := c.nextSession
	c.nextSession++
	exchangeID := c.nextExchange
	c.nextExchange++

	in, err := casesession.NewInitiator(c.fabric, c.self, peerNodeID, localSessionID)
	if err != nil {
		return nil, err
	}

	s1, err := in.Sigma1()
	if err != nil {
		return nil, err
	}
	if err := t.Send(frameUnsecured(0, exchangeID, true, message.SCCASESigma1, s1)); err != nil {
		return nil, err
	}

	proto2, sigma2, err := recvUnsecured(ctx, t)
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
	if err := t.Send(frameUnsecured(1, exchangeID, true, message.SCCASESigma3, s3)); err != nil {
		return nil, err
	}

	// The responder confirms the session with a SUCCESS StatusReport.
	proto4, sr, err := recvUnsecured(ctx, t)
	if err != nil {
		return nil, err
	}
	if proto4.Opcode != message.SCStatusReport {
		return nil, fmt.Errorf("controller: expected StatusReport, got opcode 0x%02x", proto4.Opcode)
	}
	if err := statusReportError(sr); err != nil {
		return nil, err
	}

	secure, err := in.SecureSession()
	if err != nil {
		return nil, err
	}
	return &Session{secure: secure, t: t, exchange: exchangeID + 1}, nil
}

func recvUnsecured(ctx context.Context, t transport.Transport) (message.ProtoHeader, []byte, error) {
	frame, err := t.Receive(ctx)
	if err != nil {
		return message.ProtoHeader{}, nil, err
	}
	return parseUnsecured(frame)
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
type Session struct {
	secure   *session.Secure
	t        transport.Transport
	exchange uint16
}

func (s *Session) nextExchange() uint16 {
	e := s.exchange
	s.exchange++
	return e
}

// roundTrip encrypts and sends an IM message, then decrypts the response and
// returns its protocol header and IM payload.
func (s *Session) roundTrip(ctx context.Context, opcode byte, imPayload []byte) (message.ProtoHeader, []byte, error) {
	proto := message.ProtoHeader{
		Initiator: true, Reliable: true, Opcode: opcode,
		ExchangeID: s.nextExchange(), ProtocolID: message.ProtocolInteractionModel,
	}
	frame, err := s.secure.Encrypt(append(proto.Encode(), imPayload...))
	if err != nil {
		return message.ProtoHeader{}, nil, err
	}
	if err := s.t.Send(frame); err != nil {
		return message.ProtoHeader{}, nil, err
	}
	respFrame, err := s.t.Receive(ctx)
	if err != nil {
		return message.ProtoHeader{}, nil, err
	}
	respPayload, err := s.secure.Decrypt(respFrame)
	if err != nil {
		return message.ProtoHeader{}, nil, err
	}
	return message.DecodeProto(respPayload)
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
