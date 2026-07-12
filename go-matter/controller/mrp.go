package controller

import (
	"context"
	"errors"
	"time"

	"github.com/sh5080/go-matter/message"
	"github.com/sh5080/go-matter/session"
)

// Minimal MRP (Message Reliability Protocol, Spec 4.12) for UDP transports:
//
//   - every reliable message we receive is acknowledged, either piggybacked on
//     our reply in the same exchange or with a standalone ack (SC opcode 0x10);
//   - every reliable message we send is retransmitted on a backoff until the
//     expected reply arrives or attempts run out;
//   - an authenticated duplicate (session.ErrReplay) means the peer lost our
//     acknowledgement: we re-send it and keep waiting instead of failing.
//
// Not implemented: MRP timers detached from a pending reply (we only
// retransmit while waiting for something) and BACKOFF_MARGIN jitter. Both are
// acceptable for a controller whose messages all expect replies.
const (
	mrpMaxRetransmits  = 4 // in addition to the initial transmission
	mrpInitialInterval = 400 * time.Millisecond
	mrpBackoffBase     = 2 // interval doubles per retransmission
	mrpIntervalCeiling = 3200 * time.Millisecond

	// maxExchangeMessages bounds how many non-reply messages (duplicates,
	// standalone acks, foreign-exchange frames) a single reliable exchange will
	// tolerate before giving up — an anti-flood/anti-reflection cap. Legitimate
	// exchanges need only a few iterations.
	maxExchangeMessages = 64
)

// mrpInterval returns the wait-before-retransmit for the given retransmission
// count (0 = after the initial send).
func mrpInterval(retransmits int) time.Duration {
	iv := mrpInitialInterval
	for i := 0; i < retransmits && iv < mrpIntervalCeiling; i++ {
		iv *= mrpBackoffBase
	}
	if iv > mrpIntervalCeiling {
		iv = mrpIntervalCeiling
	}
	return iv
}

// secureMsg is one decrypted message from the session's peer.
type secureMsg struct {
	ph      message.ProtoHeader
	counter uint32 // message counter, referenced by MRP acks
	payload []byte // protocol payload (after the protocol header)
}

// encryptProto seals a protocol header + payload into a wire frame.
func (s *Session) encryptProto(proto message.ProtoHeader, payload []byte) ([]byte, error) {
	return s.secure.Encrypt(append(proto.Encode(), payload...))
}

// recvMsg receives and decrypts one message. On an authenticated duplicate the
// message is returned together with session.ErrReplay.
func (s *Session) recvMsg(ctx context.Context) (secureMsg, error) {
	frame, err := s.t.Receive(ctx)
	if err != nil {
		return secureMsg{}, err
	}
	counter, payload, derr := s.secure.DecryptMsg(frame)
	if derr != nil && !errors.Is(derr, session.ErrReplay) {
		return secureMsg{}, derr
	}
	ph, rest, err := message.DecodeProto(payload)
	if err != nil {
		return secureMsg{}, err
	}
	return secureMsg{ph: ph, counter: counter, payload: rest}, derr
}

// ackMsg sends a standalone MRP ack for m. The initiator flag is ours-in-that-
// exchange: the complement of the sender's.
func (s *Session) ackMsg(m secureMsg) error {
	proto := message.ProtoHeader{
		Initiator: !m.ph.Initiator, Opcode: message.SCStandaloneAck,
		ExchangeID: m.ph.ExchangeID, ProtocolID: message.ProtocolSecureChannel,
		AckPresent: true, AckCounter: m.counter,
	}
	frame, err := s.encryptProto(proto, nil)
	if err != nil {
		return err
	}
	return s.t.Send(frame)
}

// isStandaloneAck reports whether m is an MRP standalone ack.
func isStandaloneAck(ph message.ProtoHeader) bool {
	return ph.ProtocolID == message.ProtocolSecureChannel && ph.Opcode == message.SCStandaloneAck
}

// exchangeRT waits for the peer's next protocol message in exchangeID,
// retransmitting sentFrame (our last reliable message in that exchange) on
// timeout, re-answering duplicates, and skipping standalone acks. It is the
// receive half of a reliable request/response exchange.
func (s *Session) exchangeRT(ctx context.Context, sentFrame []byte, exchangeID uint16) (secureMsg, error) {
	retransmits := 0
	for iters := 0; ; iters++ {
		// Honor the caller's context on every iteration and bound the number of
		// non-reply messages we will process. Without this, a chatty or hostile
		// peer that keeps the socket fed (duplicates, standalone acks, or frames
		// for other exchanges) would loop forever — reflecting a frame per
		// injected packet and defeating the caller's timeout. A legitimate
		// exchange resolves in a handful of iterations.
		if err := ctx.Err(); err != nil {
			return secureMsg{}, err
		}
		if iters > maxExchangeMessages {
			return secureMsg{}, errors.New("controller: too many messages without a matching reply")
		}

		actx, cancel := context.WithTimeout(ctx, mrpInterval(retransmits))
		m, err := s.recvMsg(actx)
		cancel()

		switch {
		case err == nil:
			// fall through to classification below

		case errors.Is(err, session.ErrReplay):
			// The peer retransmitted: it lost an acknowledgement of ours. If the
			// duplicate belongs to this exchange, our own last frame carries the
			// (piggybacked) ack — resend it; otherwise a standalone ack suffices.
			if m.ph.ExchangeID == exchangeID {
				if serr := s.t.Send(sentFrame); serr != nil {
					return secureMsg{}, serr
				}
			} else if aerr := s.ackMsg(m); aerr != nil {
				return secureMsg{}, aerr
			}
			continue

		case ctx.Err() != nil:
			return secureMsg{}, ctx.Err()

		case errors.Is(err, context.DeadlineExceeded):
			if retransmits >= mrpMaxRetransmits {
				return secureMsg{}, errors.New("controller: no response after MRP retransmissions")
			}
			if serr := s.t.Send(sentFrame); serr != nil {
				return secureMsg{}, serr
			}
			retransmits++
			continue

		default:
			return secureMsg{}, err
		}

		if isStandaloneAck(m.ph) {
			continue // transport-level ack of our message; the reply is still coming
		}
		if m.ph.ExchangeID != exchangeID {
			// Late traffic from an earlier exchange (e.g. a re-sent response whose
			// counter is still fresh). Ack it so the peer stops, and keep waiting.
			if aerr := s.ackMsg(m); aerr != nil {
				return secureMsg{}, aerr
			}
			continue
		}
		return m, nil
	}
}
