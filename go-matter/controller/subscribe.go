package controller

import (
	"context"
	"fmt"

	"github.com/sh5080/go-matter/im"
	"github.com/sh5080/go-matter/message"
)

// sendIM encrypts and sends an IM message on the given exchange.
func (s *Session) sendIM(opcode byte, exchange uint16, imPayload []byte) error {
	proto := message.ProtoHeader{
		Initiator: true, Reliable: true, Opcode: opcode,
		ExchangeID: exchange, ProtocolID: message.ProtocolInteractionModel,
	}
	frame, err := s.secure.Encrypt(append(proto.Encode(), imPayload...))
	if err != nil {
		return err
	}
	return s.t.Send(frame)
}

// recvIM receives, decrypts, and decodes one IM message.
func (s *Session) recvIM(ctx context.Context) (message.ProtoHeader, []byte, error) {
	frame, err := s.t.Receive(ctx)
	if err != nil {
		return message.ProtoHeader{}, nil, err
	}
	payload, err := s.secure.Decrypt(frame)
	if err != nil {
		return message.ProtoHeader{}, nil, err
	}
	return message.DecodeProto(payload)
}

// Subscription is an active attribute subscription to a device.
//
// A subscription takes ownership of the session's receive path (reports arrive
// unsolicited), so a session that is being Listen'd must not be used to Invoke
// or Read concurrently — dedicate a session to the subscription.
type Subscription struct {
	sess        *Session
	ID          uint32
	MaxInterval uint16               // seconds; device reports at least this often
	Initial     []im.AttributeReport // attribute values from the priming report
}

// Subscribe establishes a subscription to paths. minFloor/maxCeiling bound the
// device's reporting interval in seconds. It performs the full setup handshake
// (SubscribeRequest → priming ReportData → StatusResponse → SubscribeResponse)
// and returns the subscription with its initial attribute values.
func (s *Session) Subscribe(ctx context.Context, paths []im.AttributePath, minFloor, maxCeiling uint16) (*Subscription, error) {
	req, err := im.EncodeSubscribeRequest(paths, minFloor, maxCeiling, false, true)
	if err != nil {
		return nil, err
	}
	ex := s.nextExchange()
	if err := s.sendIM(message.IMSubscribeRequest, ex, req); err != nil {
		return nil, err
	}

	// The device sends the priming ReportData with current values.
	ph, payload, err := s.recvIM(ctx)
	if err != nil {
		return nil, err
	}
	if ph.Opcode != message.IMReportData {
		return nil, fmt.Errorf("controller: expected priming ReportData, got opcode 0x%02x", ph.Opcode)
	}
	_, reports, err := im.DecodeReportData(payload)
	if err != nil {
		return nil, err
	}

	// Acknowledge the report so the device finalizes the subscription.
	if err := s.ackReport(ex); err != nil {
		return nil, err
	}

	// The device confirms with a SubscribeResponse (subscription id + interval).
	ph, payload, err = s.recvIM(ctx)
	if err != nil {
		return nil, err
	}
	if ph.Opcode != message.IMSubscribeResponse {
		return nil, fmt.Errorf("controller: expected SubscribeResponse, got opcode 0x%02x", ph.Opcode)
	}
	subID, maxInterval, err := im.DecodeSubscribeResponse(payload)
	if err != nil {
		return nil, err
	}
	return &Subscription{sess: s, ID: subID, MaxInterval: maxInterval, Initial: reports}, nil
}

// ackReport sends a SUCCESS StatusResponse on exchange to acknowledge a report.
func (s *Session) ackReport(exchange uint16) error {
	ack, err := im.EncodeStatusResponse(im.StatusSuccess)
	if err != nil {
		return err
	}
	return s.sendIM(message.IMStatusResponse, exchange, ack)
}
