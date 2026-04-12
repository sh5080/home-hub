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
	hdr := message.Header{SessionType: message.Unicast, Counter: counter} // SessionID 0 = unsecured
	aad, _ := hdr.Encode()
	proto := message.ProtoHeader{
		Initiator: initiator, Opcode: opcode, ExchangeID: exchangeID,
		ProtocolID: message.ProtocolSecureChannel,
	}
	out := append(aad, proto.Encode()...)
	return append(out, payload...)
}

// parseUnsecured parses an unsecured message, returning its protocol header and
// payload.
func parseUnsecured(frame []byte) (message.ProtoHeader, []byte, error) {
	hdr, rest, err := message.Decode(frame)
	if err != nil {
		return message.ProtoHeader{}, nil, err
	}
	if hdr.SessionID != 0 {
		return message.ProtoHeader{}, nil, fmt.Errorf("controller: expected unsecured message, got session %d", hdr.SessionID)
	}
	return message.DecodeProto(rest)
}
