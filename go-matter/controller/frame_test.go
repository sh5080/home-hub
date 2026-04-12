package controller

import (
	"bytes"
	"testing"

	"github.com/sh5080/go-matter/message"
)

func TestFrameUnsecuredRoundTrip(t *testing.T) {
	payload := []byte("sigma1-bytes")
	frame := frameUnsecured(5, 0x1234, true, message.SCCASESigma1, payload)

	proto, got, err := parseUnsecured(frame)
	if err != nil {
		t.Fatal(err)
	}
	if proto.ProtocolID != message.ProtocolSecureChannel || proto.Opcode != message.SCCASESigma1 {
		t.Fatalf("proto = %+v", proto)
	}
	if proto.ExchangeID != 0x1234 || !proto.Initiator {
		t.Fatalf("exchange/initiator wrong: %+v", proto)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload = %q", got)
	}
}

func TestParseUnsecuredRejectsSecured(t *testing.T) {
	// A message with a non-zero session id is a secured message, not a handshake.
	hdr := message.Header{SessionID: 7, Counter: 1}
	aad, _ := hdr.Encode()
	if _, _, err := parseUnsecured(append(aad, 0, 0, 0, 0, 0, 0)); err == nil {
		t.Fatal("expected rejection of a secured message")
	}
}
