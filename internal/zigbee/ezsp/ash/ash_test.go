package ash

import (
	"bytes"
	"testing"
)

// crc16 vectors from the ASH spec framing examples.
func TestCRC16(t *testing.T) {
	// RST frame body {0xC0} -> CRC 0x38BC (frame on wire: C0 38 BC 7E).
	if got := crc16([]byte{0xC0}); got != 0x38BC {
		t.Fatalf("crc16(RST) = %#04x, want 0x38BC", got)
	}
	// RSTACK body {0xC1,0x02,0x02} -> CRC 0x9B7B (C1 02 02 9B 7B 7E).
	if got := crc16([]byte{0xC1, 0x02, 0x02}); got != 0x9B7B {
		t.Fatalf("crc16(RSTACK) = %#04x, want 0x9B7B", got)
	}
}

func TestRstFrameBytes(t *testing.T) {
	// Documented RST frame on the wire.
	if got := RstFrame(); !bytes.Equal(got, []byte{0xC0, 0x38, 0xBC, 0x7E}) {
		t.Fatalf("RstFrame = % X, want C0 38 BC 7E", got)
	}
}

func TestDecodeRSTACK(t *testing.T) {
	// The stuffed body between flags for a NCP reset-ack (power-on, code 0x02).
	f, err := Decode([]byte{0xC1, 0x02, 0x02, 0x9B, 0x7B})
	if err != nil {
		t.Fatalf("decode RSTACK: %v", err)
	}
	if f.Type != RSTACK || f.Code != 0x02 {
		t.Fatalf("RSTACK = %+v", f)
	}
}

func TestRandomizeIsInvolution(t *testing.T) {
	// randomize is its own inverse, so encode/decode of a DATA payload round-trips.
	in := []byte{0x00, 0x01, 0x02, 0xFF, 0xA5, 0x5A}
	if got := randomize(randomize(in)); !bytes.Equal(got, in) {
		t.Fatalf("randomize not an involution: % X", got)
	}
	// First few sequence bytes are fixed by the 0x42 seed; a zero input reveals
	// the raw sequence.
	seq := randomize([]byte{0, 0, 0, 0, 0})
	if seq[0] != 0x42 {
		t.Fatalf("sequence[0] = %#02x, want 0x42", seq[0])
	}
}

func TestByteStuffing(t *testing.T) {
	// Each reserved byte becomes Escape | (b^0x20); others pass through.
	in := []byte{0x7E, 0x11, 0x42, 0x7D, 0x13, 0x18, 0x1A}
	stuffed := stuff(in)
	if got := unstuff(stuffed); !bytes.Equal(got, in) {
		t.Fatalf("stuff/unstuff round-trip: % X", got)
	}
	// 0x7E -> 7D 5E, 0x42 stays.
	if !bytes.Equal(stuff([]byte{0x7E, 0x42}), []byte{0x7D, 0x5E, 0x42}) {
		t.Fatalf("stuff(7E 42) = % X", stuff([]byte{0x7E, 0x42}))
	}
}

func TestDataFrameRoundTrip(t *testing.T) {
	ezsp := []byte{0x00, 0x00, 0x00, 0x02} // e.g. an EZSP "version" command body
	frame := DataFrame(3, 5, ezsp)
	// Frame ends with the flag; strip it before decoding.
	if frame[len(frame)-1] != Flag {
		t.Fatalf("frame not flag-terminated")
	}
	f, err := Decode(frame[:len(frame)-1])
	if err != nil {
		t.Fatalf("decode DATA: %v", err)
	}
	if f.Type != DATA || f.FrmNum != 3 || f.AckNum != 5 || f.ReTx {
		t.Fatalf("DATA header = %+v", f)
	}
	if !bytes.Equal(f.Payload, ezsp) {
		t.Fatalf("payload = % X, want % X", f.Payload, ezsp)
	}
}

func TestDecodeAckNak(t *testing.T) {
	for _, tc := range []struct {
		frame []byte
		typ   Type
		ack   uint8
	}{
		{AckFrame(2), ACK, 2},
		{NakFrame(6), NAK, 6},
	} {
		f, err := Decode(tc.frame[:len(tc.frame)-1])
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if f.Type != tc.typ || f.AckNum != tc.ack {
			t.Fatalf("frame = %+v, want type %d ack %d", f, tc.typ, tc.ack)
		}
	}
}

func TestDecodeBadCRC(t *testing.T) {
	if _, err := Decode([]byte{0xC0, 0x00, 0x00}); err != ErrCRC {
		t.Fatalf("bad CRC err = %v, want ErrCRC", err)
	}
}
