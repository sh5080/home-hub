package pase

import "testing"

// The Matter SDK's well-known test onboarding payload: passcode 20202021,
// discriminator 0xF00 (3840), VID 0xFFF1, PID 0x8001. Its manual code and QR
// forms appear throughout connectedhomeip documentation and chip-tool.
const (
	tvManualCode = "34970112332"
	tvQRCode     = "MT:Y.K9042C00KA0648G00"
	tvPasscode   = uint32(20202021)
	tvDisc       = uint16(0xF00)
)

func TestParseManualCode(t *testing.T) {
	p, err := ParseManualCode(tvManualCode)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.Passcode != tvPasscode {
		t.Fatalf("passcode = %d, want %d", p.Passcode, tvPasscode)
	}
	if p.HasLongDiscriminator {
		t.Fatal("manual code cannot carry a long discriminator")
	}
	if p.ShortDiscriminator() != uint8(tvDisc>>8) {
		t.Fatalf("short discriminator = %d, want %d", p.ShortDiscriminator(), tvDisc>>8)
	}

	// Grouped form must parse identically.
	if p2, err := ParseManualCode("3497-011-2332"); err != nil || p2.Passcode != tvPasscode {
		t.Fatalf("grouped code: %+v (%v)", p2, err)
	}
}

func TestParseManualCodeRejectsCorruption(t *testing.T) {
	// Flip one digit: the Verhoeff check digit must catch it.
	if _, err := ParseManualCode("34970112333"); err == nil {
		t.Fatal("corrupted check digit accepted")
	}
	if _, err := ParseManualCode("35970112332"); err == nil {
		t.Fatal("corrupted payload digit accepted")
	}
	if _, err := ParseManualCode("123"); err == nil {
		t.Fatal("wrong length accepted")
	}
	if _, err := ParseManualCode("3497011233X"); err == nil {
		t.Fatal("non-digit accepted")
	}
}

func TestParseQRCode(t *testing.T) {
	p, err := ParseQRCode(tvQRCode)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.Passcode != tvPasscode {
		t.Fatalf("passcode = %d, want %d", p.Passcode, tvPasscode)
	}
	if !p.HasLongDiscriminator || p.Discriminator != tvDisc {
		t.Fatalf("discriminator = %#x (long=%v), want %#x", p.Discriminator, p.HasLongDiscriminator, tvDisc)
	}
	// VID/PID of the SDK test payload: 0xFFF1 / 0x8000. (Verifiable by hand:
	// base38 group "Y.K90" = 524168 = 0x07FF88 → PID bit0 (payload bit 19) = 0.)
	if p.VendorID != 0xFFF1 || p.ProductID != 0x8000 {
		t.Fatalf("vid/pid = %#x/%#x", p.VendorID, p.ProductID)
	}
	if p.ShortDiscriminator() != 0xF {
		t.Fatalf("short discriminator = %d", p.ShortDiscriminator())
	}
}

func TestParseQRCodeRejectsGarbage(t *testing.T) {
	if _, err := ParseQRCode("Y.K9042C00KA0648G00"); err == nil {
		t.Fatal("missing MT: prefix accepted")
	}
	if _, err := ParseQRCode("MT:!!!!"); err == nil {
		t.Fatal("invalid base38 accepted")
	}
	if _, err := ParseQRCode("MT:00"); err == nil {
		t.Fatal("truncated payload accepted")
	}
}

func TestParseOnboardingDispatch(t *testing.T) {
	if p, err := ParseOnboarding(" " + tvQRCode + " "); err != nil || !p.HasLongDiscriminator {
		t.Fatalf("QR dispatch: %+v (%v)", p, err)
	}
	if p, err := ParseOnboarding(tvManualCode); err != nil || p.HasLongDiscriminator {
		t.Fatalf("manual dispatch: %+v (%v)", p, err)
	}
}
