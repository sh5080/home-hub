package pase

import (
	"fmt"
	"strings"
)

// OnboardingPayload is the device information carried by a QR code or manual
// pairing code (Spec 5.1). A manual code carries only the SHORT (4-bit)
// discriminator; a QR code carries the full 12 bits plus vendor/product ids.
type OnboardingPayload struct {
	Passcode             uint32
	Discriminator        uint16 // 12-bit when HasLongDiscriminator, else 4-bit short
	HasLongDiscriminator bool

	// QR-only fields (zero for manual codes).
	VendorID              uint16
	ProductID             uint16
	CommissioningFlow     uint8
	DiscoveryCapabilities uint8
}

// ShortDiscriminator returns the 4-bit short discriminator used by the
// _S<n> mDNS subtype.
func (p OnboardingPayload) ShortDiscriminator() uint8 {
	if p.HasLongDiscriminator {
		return uint8(p.Discriminator >> 8)
	}
	return uint8(p.Discriminator)
}

// ParseOnboarding accepts either form: "MT:..." QR content or an 11/21-digit
// manual pairing code (whitespace and dashes tolerated).
func ParseOnboarding(s string) (OnboardingPayload, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "MT:") {
		return ParseQRCode(s)
	}
	return ParseManualCode(s)
}

// --- Manual pairing code (Spec 5.1.4.1) ---

// ParseManualCode decodes an 11-digit (or 21-digit, VID/PID-bearing) manual
// pairing code:
//
//	digit  1     : vidPidPresent(1 bit) << 2 | discriminator[11:10]
//	digits 2..6  : discriminator[9:8] << 14 | passcode[13:0]
//	digits 7..10 : passcode[26:14]
//	digit  11    : Verhoeff check digit
func ParseManualCode(code string) (OnboardingPayload, error) {
	var digits []int
	for _, r := range code {
		switch {
		case r >= '0' && r <= '9':
			digits = append(digits, int(r-'0'))
		case r == '-' || r == ' ':
			// grouping characters are allowed
		default:
			return OnboardingPayload{}, fmt.Errorf("pase: invalid character %q in manual code", r)
		}
	}
	if len(digits) != 11 && len(digits) != 21 {
		return OnboardingPayload{}, fmt.Errorf("pase: manual code must have 11 or 21 digits, got %d", len(digits))
	}
	if !verhoeffValid(digits) {
		return OnboardingPayload{}, fmt.Errorf("pase: manual code check digit mismatch")
	}

	d1 := digits[0]
	var chunk2, chunk3 uint32
	for _, d := range digits[1:6] {
		chunk2 = chunk2*10 + uint32(d)
	}
	for _, d := range digits[6:10] {
		chunk3 = chunk3*10 + uint32(d)
	}
	// Bit-width bounds: digit1 holds 3 bits, chunk2 16 bits, chunk3 13 bits.
	if d1 > 7 || chunk2 > 0xFFFF || chunk3 > 0x1FFF {
		return OnboardingPayload{}, fmt.Errorf("pase: manual code field out of range")
	}

	passcode := (chunk3 << 14) | (chunk2 & 0x3FFF)
	shortDisc := uint16(d1&0x3)<<2 | uint16((chunk2>>14)&0x3)
	if passcode == 0 || passcode > 99999998 {
		return OnboardingPayload{}, fmt.Errorf("pase: passcode %d out of range", passcode)
	}
	return OnboardingPayload{
		Passcode:      passcode,
		Discriminator: shortDisc, // 4-bit short discriminator
	}, nil
}

// Verhoeff checksum tables (dihedral group D5).
var verhoeffD = [10][10]int{
	{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
	{1, 2, 3, 4, 0, 6, 7, 8, 9, 5},
	{2, 3, 4, 0, 1, 7, 8, 9, 5, 6},
	{3, 4, 0, 1, 2, 8, 9, 5, 6, 7},
	{4, 0, 1, 2, 3, 9, 5, 6, 7, 8},
	{5, 9, 8, 7, 6, 0, 4, 3, 2, 1},
	{6, 5, 9, 8, 7, 1, 0, 4, 3, 2},
	{7, 6, 5, 9, 8, 2, 1, 0, 4, 3},
	{8, 7, 6, 5, 9, 3, 2, 1, 0, 4},
	{9, 8, 7, 6, 5, 4, 3, 2, 1, 0},
}

var verhoeffP = [8][10]int{
	{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
	{1, 5, 7, 6, 2, 8, 3, 0, 9, 4},
	{5, 8, 0, 3, 7, 9, 6, 1, 4, 2},
	{8, 9, 1, 6, 0, 4, 3, 5, 2, 7},
	{9, 4, 5, 3, 1, 2, 6, 8, 7, 0},
	{4, 2, 8, 6, 5, 7, 3, 9, 0, 1},
	{2, 7, 9, 3, 8, 0, 6, 4, 1, 5},
	{7, 0, 4, 6, 9, 1, 3, 2, 5, 8},
}

// verhoeffValid checks the trailing Verhoeff check digit over the payload.
func verhoeffValid(digits []int) bool {
	c := 0
	// Process from the right, check digit included (position 0).
	for i := 0; i < len(digits); i++ {
		d := digits[len(digits)-1-i]
		c = verhoeffD[c][verhoeffP[i%8][d]]
	}
	return c == 0
}

// --- QR code payload (Spec 5.1.3) ---

const base38Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ-."

// ParseQRCode decodes an "MT:" QR payload: base38 → packed little-endian bit
// fields: version(3) vendorID(16) productID(16) flow(2) capabilities(8)
// discriminator(12) passcode(27) padding(4).
func ParseQRCode(qr string) (OnboardingPayload, error) {
	body, ok := strings.CutPrefix(qr, "MT:")
	if !ok {
		return OnboardingPayload{}, fmt.Errorf("pase: QR payload must start with MT:")
	}
	raw, err := base38Decode(body)
	if err != nil {
		return OnboardingPayload{}, err
	}
	if len(raw) < 11 {
		return OnboardingPayload{}, fmt.Errorf("pase: QR payload too short (%d bytes)", len(raw))
	}
	br := bitReader{data: raw}
	version := br.read(3)
	vid := br.read(16)
	pid := br.read(16)
	flow := br.read(2)
	caps := br.read(8)
	disc := br.read(12)
	passcode := br.read(27)
	if br.err != nil {
		return OnboardingPayload{}, br.err
	}
	if version != 0 {
		return OnboardingPayload{}, fmt.Errorf("pase: unsupported QR payload version %d", version)
	}
	if passcode == 0 || passcode > 99999998 {
		return OnboardingPayload{}, fmt.Errorf("pase: passcode %d out of range", passcode)
	}
	return OnboardingPayload{
		Passcode:              uint32(passcode),
		Discriminator:         uint16(disc),
		HasLongDiscriminator:  true,
		VendorID:              uint16(vid),
		ProductID:             uint16(pid),
		CommissioningFlow:     uint8(flow),
		DiscoveryCapabilities: uint8(caps),
	}, nil
}

// base38Decode reverses Matter's base38: groups of 5 chars → 3 bytes, with a
// final group of 4 chars → 2 bytes or 2 chars → 1 byte.
func base38Decode(s string) ([]byte, error) {
	val := func(r byte) (uint32, error) {
		i := strings.IndexByte(base38Alphabet, r)
		if i < 0 {
			return 0, fmt.Errorf("pase: invalid base38 character %q", r)
		}
		return uint32(i), nil
	}
	var out []byte
	for i := 0; i < len(s); {
		rem := len(s) - i
		var chars, bytes int
		switch {
		case rem >= 5:
			chars, bytes = 5, 3
		case rem == 4:
			chars, bytes = 4, 2
		case rem == 2:
			chars, bytes = 2, 1
		default:
			return nil, fmt.Errorf("pase: invalid base38 length remainder %d", rem)
		}
		var v uint32
		for j := chars - 1; j >= 0; j-- {
			d, err := val(s[i+j])
			if err != nil {
				return nil, err
			}
			v = v*38 + d
		}
		for j := 0; j < bytes; j++ {
			out = append(out, byte(v))
			v >>= 8
		}
		i += chars
	}
	return out, nil
}

// bitReader reads little-endian bit fields from a byte slice (bit 0 of byte 0
// first), as the QR payload packs them.
type bitReader struct {
	data []byte
	pos  int
	err  error
}

func (b *bitReader) read(n int) uint32 {
	if b.err != nil {
		return 0
	}
	var v uint32
	for i := 0; i < n; i++ {
		byteIdx, bitIdx := b.pos/8, b.pos%8
		if byteIdx >= len(b.data) {
			b.err = fmt.Errorf("pase: QR payload truncated")
			return 0
		}
		if b.data[byteIdx]&(1<<bitIdx) != 0 {
			v |= 1 << i
		}
		b.pos++
	}
	return v
}
