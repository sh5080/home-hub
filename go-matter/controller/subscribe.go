package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/sh5080/go-matter/im"
	"github.com/sh5080/go-matter/message"
	"github.com/sh5080/go-matter/session"
)

// Subscription is an active attribute subscription to a device.
//
// A subscription takes ownership of the session's receive path (reports arrive
// unsolicited), so a session that is being Listen'd must not be used to Invoke
// or Read concurrently — dedicate a session to the subscription. Listen holds
// the session mutex for its lifetime to enforce this.
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
	s.mu.Lock()
	defer s.mu.Unlock()

	req, err := im.EncodeSubscribeRequest(paths, minFloor, maxCeiling, false, true)
	if err != nil {
		return nil, err
	}
	ex := s.nextExchange()
	reqFrame, err := s.encryptProto(message.ProtoHeader{
		Initiator: true, Reliable: true, Opcode: message.IMSubscribeRequest,
		ExchangeID: ex, ProtocolID: message.ProtocolInteractionModel,
	}, req)
	if err != nil {
		return nil, err
	}
	if err := s.t.Send(reqFrame); err != nil {
		return nil, err
	}

	// The device sends the priming ReportData with current values.
	prime, err := s.exchangeRT(ctx, reqFrame, ex)
	if err != nil {
		return nil, err
	}
	if prime.ph.Opcode != message.IMReportData {
		return nil, fmt.Errorf("controller: expected priming ReportData, got opcode 0x%02x", prime.ph.Opcode)
	}
	_, reports, err := im.DecodeReportData(prime.payload)
	if err != nil {
		return nil, err
	}

	// Acknowledge the report (IM StatusResponse carrying the MRP ack) so the
	// device finalizes the subscription. This is our exchange → Initiator true.
	srFrame, err := s.statusResponseFrame(ex, true, prime.counter)
	if err != nil {
		return nil, err
	}
	if err := s.t.Send(srFrame); err != nil {
		return nil, err
	}

	// The device confirms with a SubscribeResponse (subscription id + interval).
	resp, err := s.exchangeRT(ctx, srFrame, ex)
	if err != nil {
		return nil, err
	}
	if resp.ph.Opcode != message.IMSubscribeResponse {
		return nil, fmt.Errorf("controller: expected SubscribeResponse, got opcode 0x%02x", resp.ph.Opcode)
	}
	subID, maxInterval, err := im.DecodeSubscribeResponse(resp.payload)
	if err != nil {
		return nil, err
	}
	if err := s.ackMsg(resp); err != nil {
		return nil, err
	}
	return &Subscription{sess: s, ID: subID, MaxInterval: maxInterval, Initial: reports}, nil
}

// statusResponseFrame builds an encrypted IM StatusResponse(SUCCESS) on the
// given exchange, piggybacking the MRP ack of ackCounter. initiator is our role
// in that exchange: true when we opened it (subscription setup), false when the
// device did (subsequent reports).
func (s *Session) statusResponseFrame(exchangeID uint16, initiator bool, ackCounter uint32) ([]byte, error) {
	payload, err := im.EncodeStatusResponse(im.StatusSuccess)
	if err != nil {
		return nil, err
	}
	return s.encryptProto(message.ProtoHeader{
		Initiator: initiator, Reliable: true, Opcode: message.IMStatusResponse,
		ExchangeID: exchangeID, ProtocolID: message.ProtocolInteractionModel,
		AckPresent: true, AckCounter: ackCounter,
	}, payload)
}

// Listen delivers each subsequent report to onReport until ctx is cancelled or
// the transport closes, acknowledging every report with a StatusResponse. It
// holds the session for its whole lifetime (see Subscription docs), so run it
// on a session dedicated to this subscription.
func (sub *Subscription) Listen(ctx context.Context, onReport func([]im.AttributeReport)) error {
	s := sub.sess
	s.mu.Lock()
	defer s.mu.Unlock()

	var lastReportCounter uint32
	var lastResponse []byte // our StatusResponse to the last report, for re-sends
	for {
		m, err := s.recvMsg(ctx)
		if errors.Is(err, session.ErrReplay) {
			// The device retransmitted: our ack/StatusResponse was lost.
			if lastResponse != nil && m.counter == lastReportCounter {
				if serr := s.t.Send(lastResponse); serr != nil {
					return serr
				}
			} else if aerr := s.ackMsg(m); aerr != nil {
				return aerr
			}
			continue
		}
		if err != nil {
			return err
		}
		if isStandaloneAck(m.ph) {
			continue // ack of our previous StatusResponse
		}
		if m.ph.Opcode != message.IMReportData {
			// Unexpected message on a dedicated subscription session; ack it so
			// the device stops retransmitting, and keep listening.
			if aerr := s.ackMsg(m); aerr != nil {
				return aerr
			}
			continue
		}
		_, reports, err := im.DecodeReportData(m.payload)
		if err != nil {
			return err
		}
		// Reports arrive on device-initiated exchanges → our messages in them
		// carry Initiator=false. Respond before invoking the callback so a slow
		// consumer cannot stall the subscription protocol.
		frame, err := s.statusResponseFrame(m.ph.ExchangeID, false, m.counter)
		if err != nil {
			return err
		}
		if err := s.t.Send(frame); err != nil {
			return err
		}
		lastReportCounter, lastResponse = m.counter, frame
		onReport(reports)
	}
}
