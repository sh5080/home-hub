// Package controller ties the go-matter layers into a high-level operational
// client: it runs CASE over a transport and exposes Invoke/Read on the
// resulting secure session. This is the API the hub's GoMatterDriver uses.
package controller

import (
	"fmt"

	"github.com/sh5080/go-matter/message"
)

// frameUnsecured builds an unsecured Matter message (session id 0) carrying a
// Secure Channel protocol message such as a CASE Sigma. The handshake messages
// are not encrypted, so they travel as unsecured messages.
func frameUnsecured(counter uint32, exchangeID uint16, initiator bool, opcode byte, payload []byte) []byte {
	return frameUnsecuredAck(counter, exchangeID, initiator, opcode, payload, 0, false)
}

// frameUnsecuredAck is frameUnsecured with an optional piggybacked MRP
// acknowledgement of the peer's message counter ackCounter.
func frameUnsecuredAck(counter uint32, exchangeID uint16, initiator bool, opcode byte, payload []byte, ackCounter uint32, ack bool) []byte {
	hdr := message.Header{SessionType: message.Unicast, Counter: counter} // SessionID 0 = unsecured
	aad, _ := hdr.Encode()
	proto := message.ProtoHeader{
		Initiator: initiator, Reliable: true, Opcode: opcode, ExchangeID: exchangeID,
		ProtocolID: message.ProtocolSecureChannel,
		AckCounter: ackCounter, AckPresent: ack,
	}
	if opcode == message.SCStandaloneAck {
		proto.Reliable = false // acks are never themselves reliable
	}
	out := append(aad, proto.Encode()...)
	return append(out, payload...)
}

// parseUnsecured parses an unsecured message, returning its protocol header and
// payload.
func parseUnsecured(frame []byte) (message.ProtoHeader, []byte, error) {
	_, ph, payload, err := parseUnsecuredMsg(frame)
	return ph, payload, err
}

// parseUnsecuredMsg additionally returns the message header (for its counter,
// which MRP acknowledgements reference).
func parseUnsecuredMsg(frame []byte) (message.Header, message.ProtoHeader, []byte, error) {
	hdr, rest, err := message.Decode(frame)
	if err != nil {
		return message.Header{}, message.ProtoHeader{}, nil, err
	}
	if hdr.SessionID != 0 {
		return message.Header{}, message.ProtoHeader{}, nil, fmt.Errorf("controller: expected unsecured message, got session %d", hdr.SessionID)
	}
	ph, payload, err := message.DecodeProto(rest)
	return hdr, ph, payload, err
}
