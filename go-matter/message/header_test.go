package message

import (
	"bytes"
	"testing"
)

func TestHeaderRoundTrip(t *testing.T) {
	cases := []Header{
		{SessionID: 0, SessionType: Unicast, Counter: 1}, // unsecured, no src/dest
		{
			SessionID: 0x1234, SessionType: Unicast, Counter: 0xDEADBEEF,
			SourceNodeID: 0x1122334455667788, SourcePresent: true,
			DestNodeID: 0x0102030405060708, DestKind: DestNode,
		},
		{
			SessionID: 5, SessionType: Group, Counter: 42, Control: true,
			DestGroupID: 0xBEEF, DestKind: DestGroup,
		},
		{SessionID: 9, Privacy: true, Counter: 7, SourceNodeID: 0xABCD, SourcePresent: true},
	}
	for i, h := range cases {
		enc, err := h.Encode()
		if err != nil {
			t.Fatalf("case %d encode: %v", i, err)
		}
		got, rest, err := Decode(append(enc, 0xAA, 0xBB))
		if err != nil {
			t.Fatalf("case %d decode: %v", i, err)
		}
		if got != h {
			t.Fatalf("case %d: %+v != %+v", i, got, h)
		}
		if !bytes.Equal(rest, []byte{0xAA, 0xBB}) {
			t.Fatalf("case %d payload = %x", i, rest)
		}
	}
}

func TestHeaderTruncation(t *testing.T) {
	full := Header{SourceNodeID: 1, SourcePresent: true, DestNodeID: 2, DestKind: DestNode}
	enc, _ := full.Encode()
	for n := 0; n < len(enc); n++ {
		if _, _, err := Decode(enc[:n]); err == nil {
			t.Fatalf("truncation to %d bytes should fail", n)
		}
	}
}

func TestHeaderRejectsUnsupported(t *testing.T) {
	if _, _, err := Decode([]byte{0x10, 0, 0, 0, 0, 0, 0, 0}); err == nil {
		t.Fatal("nonzero version should fail")
	}
	if _, _, err := Decode([]byte{0x03, 0, 0, 0, 0, 0, 0, 0}); err == nil {
		t.Fatal("reserved DSIZ should fail")
	}
	if _, _, err := Decode([]byte{0x00, 0, 0, secExtensions, 0, 0, 0, 0}); err == nil {
		t.Fatal("message extensions should fail")
	}
}
