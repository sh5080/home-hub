package ezsp

import (
	"bytes"
	"testing"
)

// The extended layout is [seq][frame control 2B LE][frame id 2B LE][params].
// The frame control this host sends is 0x0100, which serialises to 00 01 —
// the high byte's format-version field is what marks the frame as extended.
func TestCommandExtendedLayout(t *testing.T) {
	got := Command(0x05, IDGetNetworkParameters, nil)
	want := []byte{0x05, 0x00, 0x01, 0x28, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("Command = % X, want % X", got, want)
	}
}

func TestCommandCarriesParams(t *testing.T) {
	got := Command(0x11, IDPermitJoining, []byte{0xFE})
	want := []byte{0x11, 0x00, 0x01, 0x22, 0x00, 0xFE}
	if !bytes.Equal(got, want) {
		t.Fatalf("Command = % X, want % X", got, want)
	}
}

// Command must not alias or grow into the caller's slice: the params backing
// array is the caller's, and an append that fit would corrupt it.
func TestCommandDoesNotAliasParams(t *testing.T) {
	params := make([]byte, 1, 64)
	params[0] = 0xAA
	frame := Command(0x01, IDSendUnicast, params)
	frame[5] = 0xBB
	if params[0] != 0xAA {
		t.Fatalf("Command wrote through to caller's params: %#02x", params[0])
	}
}

func TestDecodeFrameResponse(t *testing.T) {
	// Frame control 0x0180: response bit (0x0080) plus the extended format bit.
	raw := []byte{0x05, 0x80, 0x01, 0x28, 0x00, 0x93, 0x00}
	f, err := DecodeFrame(raw)
	if err != nil {
		t.Fatalf("DecodeFrame: %v", err)
	}
	if f.Seq != 0x05 || f.ID != IDGetNetworkParameters || f.Control != 0x0180 {
		t.Fatalf("got seq=%#02x id=%#04x control=%#04x", f.Seq, f.ID, f.Control)
	}
	if !f.IsResponse() {
		t.Fatal("IsResponse = false, want true for control 0x0180")
	}
	if !bytes.Equal(f.Params, []byte{0x93, 0x00}) {
		t.Fatalf("params = % X", f.Params)
	}
}

// A two-byte frame id means callbacks live above 0x00FF; decoding must read
// both bytes rather than truncating to the legacy single-byte id.
func TestDecodeFrameTwoByteID(t *testing.T) {
	raw := []byte{0x02, 0x90, 0x01, 0x16, 0x01}
	f, err := DecodeFrame(raw)
	if err != nil {
		t.Fatalf("DecodeFrame: %v", err)
	}
	if f.ID != 0x0116 {
		t.Fatalf("ID = %#04x, want 0x0116", f.ID)
	}
	if len(f.Params) != 0 {
		t.Fatalf("params = % X, want empty", f.Params)
	}
}

func TestDecodeFrameTooShort(t *testing.T) {
	if _, err := DecodeFrame([]byte{0x01, 0x80, 0x01, 0x28}); err == nil {
		t.Fatal("DecodeFrame accepted a 4-byte frame")
	}
}

// The version handshake stays in the legacy format; regressing it would break
// the bootstrap before the extended format can be used at all.
func TestVersionCommandStaysLegacy(t *testing.T) {
	got := VersionCommand(0x01, 13)
	want := []byte{0x01, 0x00, 0x00, 0x0D}
	if !bytes.Equal(got, want) {
		t.Fatalf("VersionCommand = % X, want % X", got, want)
	}
}

func TestDecodeVersionResponse(t *testing.T) {
	// Real ZBDongle-E reply: EZSP v13, EmberZNet (stack type 2), build 0x7440.
	raw := []byte{0x01, 0x80, 0x00, 0x0D, 0x02, 0x40, 0x74}
	v, err := DecodeVersionResponse(raw)
	if err != nil {
		t.Fatalf("DecodeVersionResponse: %v", err)
	}
	if v.ProtocolVersion != 13 || v.StackType != 2 || v.StackVersion != 0x7440 {
		t.Fatalf("got %+v", v)
	}
}
