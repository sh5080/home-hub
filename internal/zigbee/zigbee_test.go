package zigbee

import "testing"

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
	on, ok := parseOnOffReport([]byte{0x08, 0x01, 0x0a, 0x00, 0x00, 0x10, 0x01})
	if !ok || !on {
		t.Fatalf("on report: on=%v ok=%v", on, ok)
	}
	off, ok := parseOnOffReport([]byte{0x08, 0x01, 0x0a, 0x00, 0x00, 0x10, 0x00})
	if !ok || off {
		t.Fatalf("off report: off=%v ok=%v", off, ok)
	}
	if _, ok := parseOnOffReport([]byte{0x08, 0x01, 0x01}); ok {
		t.Fatal("non-report command should be rejected")
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
