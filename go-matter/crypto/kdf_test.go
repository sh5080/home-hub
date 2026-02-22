package crypto

import (
	"encoding/hex"
	"testing"
)

// TestHKDF_RFC5869 uses RFC 5869 Appendix A.1 (HKDF-SHA256, Test Case 1).
func TestHKDF_RFC5869(t *testing.T) {
	ikm := unhex(t, "0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b")
	salt := unhex(t, "000102030405060708090a0b0c")
	info := unhex(t, "f0f1f2f3f4f5f6f7f8f9")
	want := "3cb25f25faacd57a90434f64d0362f2a2d2d0a90cf1a5a4c5db02d56ecc4c5bf34007208d5b887185865"

	got, err := HKDF(ikm, salt, info, 42)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(got) != want {
		t.Fatalf("HKDF = %x, want %s", got, want)
	}
}

// TestPBKDF2_RFC7914 uses the PBKDF2-HMAC-SHA-256 test vector from RFC 7914
// Section 11.
func TestPBKDF2_RFC7914(t *testing.T) {
	want := "55ac046e56e3089fec1691c22544b605f94185216dde0465e68b9d57c20dacbc" +
		"49ca9cccf179b645991664b39d77ef317c71b845b1e30bd509112041d3a19783"

	got, err := PBKDF2([]byte("passwd"), []byte("salt"), 1, 64)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(got) != want {
		t.Fatalf("PBKDF2 = %x, want %s", got, want)
	}
}
