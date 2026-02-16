package message

import (
	"bytes"
	"testing"
)

func TestProtoHeaderRoundTrip(t *testing.T) {
	cases := []ProtoHeader{
		{Initiator: true, Opcode: SCCASESigma1, ExchangeID: 1, ProtocolID: ProtocolSecureChannel},
		{Reliable: true, Opcode: IMInvokeRequest, ExchangeID: 0xBEEF, ProtocolID: ProtocolInteractionModel},
		{Opcode: SCStandaloneAck, ExchangeID: 2, ProtocolID: ProtocolSecureChannel, AckCounter: 0x11223344, AckPresent: true},
		{Initiator: true, Opcode: IMInvokeRequest, ExchangeID: 7, ProtocolID: 0xFC01, VendorID: 0xFFF1, VendorPresent: true, AckCounter: 9, AckPresent: true},
	}
	for i, p := range cases {
		got, payload, err := DecodeProto(append(p.Encode(), 0x15, 0x18))
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if got != p {
			t.Fatalf("case %d: %+v != %+v", i, got, p)
		}
		if !bytes.Equal(payload, []byte{0x15, 0x18}) {
			t.Fatalf("case %d payload = %x", i, payload)
		}
	}
}

func TestProtoTruncation(t *testing.T) {
	p := ProtoHeader{VendorPresent: true, AckPresent: true, ProtocolID: 1}
	enc := p.Encode()
	for n := 0; n < len(enc); n++ {
		if _, _, err := DecodeProto(enc[:n]); err == nil {
			t.Fatalf("truncation to %d should fail", n)
		}
	}
}

func TestStatusReport(t *testing.T) {
	s := StatusReport{GeneralCode: GeneralSuccess, ProtocolID: uint32(ProtocolSecureChannel)}
	got, err := DecodeStatusReport(s.Encode())
	if err != nil {
		t.Fatal(err)
	}
	if got != s || !got.IsSuccess() || got.Error() != nil {
		t.Fatalf("status = %+v", got)
	}

	fail := StatusReport{GeneralCode: GeneralFailure, ProtocolID: uint32(ProtocolSecureChannel), ProtocolCode: 3}
	if fail.IsSuccess() || fail.Error() == nil {
		t.Fatal("failure report should not be success")
	}
	if _, err := DecodeStatusReport([]byte{1, 2, 3}); err == nil {
		t.Fatal("short status should fail")
	}
}
