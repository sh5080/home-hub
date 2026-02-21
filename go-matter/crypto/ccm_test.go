package crypto

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestCCM_RFC3610 validates the CBC-MAC + CTR construction (including the
// (M-2)/2 flags encoding) against RFC 3610 packet vectors #1 and #2, which use
// a 13-byte nonce and an 8-byte MAC.
func TestCCM_RFC3610(t *testing.T) {
	cases := []struct{ key, nonce, aad, plain, want string }{
		{
			key:   "c0c1c2c3c4c5c6c7c8c9cacbcccdcecf",
			nonce: "00000003020100a0a1a2a3a4a5",
			aad:   "0001020304050607",
			plain: "08090a0b0c0d0e0f101112131415161718191a1b1c1d1e",
			want:  "588c979a61c663d2f066d0c2c0f989806d5f6b61dac38417e8d12cfdf926e0",
		},
		{
			key:   "c0c1c2c3c4c5c6c7c8c9cacbcccdcecf",
			nonce: "00000004030201a0a1a2a3a4a5",
			aad:   "0001020304050607",
			plain: "08090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
			want:  "72c91a36e135f8cf291ca894085c87e3cc15c439c9e43a3ba091d56e10400916",
		},
	}
	for i, tc := range cases {
		c, err := newCCM(unhex(t, tc.key), 13, 8)
		if err != nil {
			t.Fatal(err)
		}
		got := c.Seal(nil, unhex(t, tc.nonce), unhex(t, tc.plain), unhex(t, tc.aad))
		if hex.EncodeToString(got) != tc.want {
			t.Fatalf("vector %d: seal = %x, want %s", i+1, got, tc.want)
		}
		pt, err := c.Open(nil, unhex(t, tc.nonce), got, unhex(t, tc.aad))
		if err != nil {
			t.Fatalf("vector %d open: %v", i+1, err)
		}
		if !bytes.Equal(pt, unhex(t, tc.plain)) {
			t.Fatalf("vector %d: open = %x", i+1, pt)
		}
	}
}

// TestCCM_Matter16 confirms end-to-end integrity at Matter's parameters
// (13-byte nonce, 16-byte tag). The flags formula is already pinned by the
// RFC 3610 M=8 vectors above.
func TestCCM_Matter16(t *testing.T) {
	c, err := NewCCM(unhex(t, "d7828d13b2b0bdc325a76236df93cc6b"))
	if err != nil {
		t.Fatal(err)
	}
	if c.NonceSize() != 13 || c.Overhead() != 16 {
		t.Fatalf("params: nonce=%d tag=%d", c.NonceSize(), c.Overhead())
	}
	nonce := unhex(t, "00112233445566778899aabbcc")
	aad := []byte("message header aad")
	plain := []byte("turn the blind to 37 percent")

	ct := c.Seal(nil, nonce, plain, aad)
	if len(ct) != len(plain)+16 {
		t.Fatalf("ciphertext length %d", len(ct))
	}
	pt, err := c.Open(nil, nonce, ct, aad)
	if err != nil || !bytes.Equal(pt, plain) {
		t.Fatalf("round trip failed: %v", err)
	}

	// Every kind of tampering must be rejected.
	flip := func(b []byte, i int) []byte {
		out := append([]byte(nil), b...)
		out[i] ^= 1
		return out
	}
	if _, err := c.Open(nil, nonce, flip(ct, 0), aad); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
	if _, err := c.Open(nil, nonce, flip(ct, len(ct)-1), aad); err == nil {
		t.Fatal("tampered tag accepted")
	}
	if _, err := c.Open(nil, nonce, ct, append(append([]byte(nil), aad...), 'x')); err == nil {
		t.Fatal("tampered aad accepted")
	}
	if _, err := c.Open(nil, flip(nonce, 12), ct, aad); err == nil {
		t.Fatal("wrong nonce accepted")
	}
}

func TestCCM_EmptyAAD(t *testing.T) {
	c, _ := NewCCM(make([]byte, 16))
	nonce := make([]byte, 13)
	ct := c.Seal(nil, nonce, []byte("hi"), nil)
	pt, err := c.Open(nil, nonce, ct, nil)
	if err != nil || string(pt) != "hi" {
		t.Fatalf("empty-aad round trip: %v", err)
	}
}

func TestCCM_BadKey(t *testing.T) {
	if _, err := NewCCM(make([]byte, 24)); err == nil {
		t.Fatal("192-bit key should be rejected (Matter is AES-128)")
	}
}
