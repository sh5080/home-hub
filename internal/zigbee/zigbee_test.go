package zigbee

import (
	"testing"

	"github.com/sh5080/home-hub/internal/domain"
)

func TestParseIEEE(t *testing.T) {
	a, err := parseIEEE("0x00158d0001abcd01")
	if err != nil {
		t.Fatalf("parseIEEE: %v", err)
	}
	if uint64(a) != 0x00158d0001abcd01 {
		t.Fatalf("addr = %#x", uint64(a))
	}
	if _, err := parseIEEE("nothex"); err == nil {
		t.Fatal("expected error for bad address")
	}
}

func TestParseOnOffReport(t *testing.T) {
	// frame control, seq, cmd 0x0a, attr 0x0000 LE, type 0x10 (bool), value.
	on, ok := parseAttrBool([]byte{0x08, 0x01, 0x0a, 0x00, 0x00, 0x10, 0x01}, 0x0000)
	if !ok || !on {
		t.Fatalf("on report: on=%v ok=%v", on, ok)
	}
	off, ok := parseAttrBool([]byte{0x08, 0x01, 0x0a, 0x00, 0x00, 0x10, 0x00}, 0x0000)
	if !ok || off {
		t.Fatalf("off report: off=%v ok=%v", off, ok)
	}
	if _, ok := parseAttrBool([]byte{0x08, 0x01, 0x01}, 0x0000); ok {
		t.Fatal("non-report command should be rejected")
	}
}

func TestParseCoverPosition(t *testing.T) {
	// attr 0x0008 LE, type 0x20 (uint8), value 25 (= ZCL 25% closed).
	v, ok := parseAttrUint8([]byte{0x08, 0x01, 0x0a, 0x08, 0x00, 0x20, 25}, covAttrLiftPercentage)
	if !ok || v != 25 {
		t.Fatalf("cover report: v=%d ok=%v", v, ok)
	}
	// Wrong attribute id must not match.
	if _, ok := parseAttrUint8([]byte{0x08, 0x01, 0x0a, 0x00, 0x00, 0x20, 25}, covAttrLiftPercentage); ok {
		t.Fatal("wrong attr id should be rejected")
	}
}

func TestParseMultistatePress(t *testing.T) {
	frame := func(v uint16) []byte {
		// attr 0x0055 LE, type 0x21 (uint16), value LE.
		return []byte{0x08, 0x01, 0x0a, 0x55, 0x00, 0x21, byte(v), byte(v >> 8)}
	}
	cases := []struct {
		v    uint16
		want string
	}{{1, domain.PressSingle}, {2, domain.PressDouble}, {0, domain.PressHold}}
	for _, c := range cases {
		got, ok := parseMultistatePress(frame(c.v))
		if !ok || got != c.want {
			t.Fatalf("press(%d) = %q ok=%v, want %q", c.v, got, ok, c.want)
		}
	}
	if _, ok := parseMultistatePress(frame(9)); ok {
		t.Fatal("unknown press value should be rejected")
	}
}

func TestDeviceEndpointDefault(t *testing.T) {
	if deviceEndpoint(domain.Device{}) != 1 {
		t.Fatal("default endpoint must be 1")
	}
	if deviceEndpoint(domain.Device{Endpoint: 2}) != 2 {
		t.Fatal("explicit endpoint must pass through")
	}
}

func TestLumiOpModeFrame(t *testing.T) {
	f := lumiOpModeFrame(0x07, lumiDecoupledMode)
	want := []byte{
		0x04,       // manufacturer-specific frame control
		0x5F, 0x11, // manufacturer code 0x115F, little-endian
		0x07,       // sequence
		0x02,       // Write Attributes
		0x00, 0x02, // attribute 0x0200, little-endian
		0x20, // uint8
		0x00, // decoupled
	}
	if len(f) != len(want) {
		t.Fatalf("frame len = %d, want %d", len(f), len(want))
	}
	for i := range want {
		if f[i] != want[i] {
			t.Fatalf("byte %d = %#x, want %#x (frame %#x)", i, f[i], want[i], f)
		}
	}
}

func TestIsLumi(t *testing.T) {
	if !isLumi(0x00158d0001abcd01) {
		t.Fatal("expected lumi device")
	}
	if isLumi(0x0011223344556677) {
		t.Fatal("expected non-lumi device")
	}
}

func TestAqaraOnOff(t *testing.T) {
	// report cmd, attr 0xFF01 LE, attr type, then tag 0x64 / type 0x10 / value.
	on, ok := aqaraOnOff([]byte{0x1c, 0x01, 0x0a, 0x01, 0xFF, 0x42, 0x64, 0x10, 0x01})
	if !ok || !on {
		t.Fatalf("aqara: on=%v ok=%v", on, ok)
	}
}
