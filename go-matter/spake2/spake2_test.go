package spake2

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"testing"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}

func mustBig(t *testing.T, s string) *big.Int {
	return new(big.Int).SetBytes(mustHex(t, s))
}

// TestSPAKE2Plus_RFC9383 validates the P-256 group math (shares and the shared
// points Z, V) against the RFC 9383 P256-SHA256-HKDF-HMAC worked example. Matter
// uses the identical M, N and share construction, so this pins the core.
func TestSPAKE2Plus_RFC9383(t *testing.T) {
	w0 := mustBig(t, "bb8e1bbcf3c48f62c08db243652ae55d3e5586053fca77102994f23ad95491b3")
	w1 := mustBig(t, "7e945f34d78785b8a3ef44d0df5a1a97d6b3b460409a345ca7830387a74b1dba")
	x := mustBig(t, "d1232c8e8693d02368976c174e2088851b8365d0d79a9eee709c6a05a2fad539")

	shareP := mustHex(t, "04ef3bd051bf78a2234ec0df197f7828060fe9856503579bb1733009042c15c0c1de127727f418b5966afadfdd95a6e4591d171056b333dab97a79c7193e341727")
	shareV := mustHex(t, "04c0f65da0d11927bdf5d560c69e1d7d939a05b0e88291887d679fcadea75810fb5cc1ca7494db39e82ff2f50665255d76173e09986ab46742c798a9a68437b048")
	wantZ := mustHex(t, "04bbfce7dd7f277819c8da21544afb7964705569bdf12fb92aa388059408d50091a0c5f1d3127f56813b5337f9e4e67e2ca633117a4fbd559946ab474356c41839")
	wantV := mustHex(t, "0458bf27c6bca011c9ce1930e8984a797a3419797b936629a5a937cf2f11c8b9514b82b993da8a46e664f23db7c01edc87faa530db01c2ee405230b18997f16b68")

	p, err := NewProver(w0, w1, x)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(p.Share(), shareP) {
		t.Fatalf("shareP:\n got  %x\n want %x", p.Share(), shareP)
	}
	z, v, err := p.Finish(shareV)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(z, wantZ) {
		t.Fatalf("Z:\n got  %x\n want %x", z, wantZ)
	}
	if !bytes.Equal(v, wantV) {
		t.Fatalf("V:\n got  %x\n want %x", v, wantV)
	}
}

func TestProverRejectsBadShare(t *testing.T) {
	p, err := NewProver(big.NewInt(3), big.NewInt(5), big.NewInt(7))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.Finish([]byte{0x04, 0x00}); err == nil {
		t.Fatal("invalid verifier share should be rejected")
	}
}
