package spake2

import (
	"encoding/hex"
	"testing"
)

func assertHex(t *testing.T, name string, got []byte, want string) {
	t.Helper()
	if hex.EncodeToString(got) != want {
		t.Fatalf("%s:\n got  %x\n want %s", name, got, want)
	}
}

// TestSPAKE2Plus_KeySchedule verifies the transcript, K_main, confirmation MACs,
// and shared key against the RFC 9383 P256-SHA256 worked example.
func TestSPAKE2Plus_KeySchedule(t *testing.T) {
	w0 := mustBig(t, "bb8e1bbcf3c48f62c08db243652ae55d3e5586053fca77102994f23ad95491b3")
	w1 := mustBig(t, "7e945f34d78785b8a3ef44d0df5a1a97d6b3b460409a345ca7830387a74b1dba")
	x := mustBig(t, "d1232c8e8693d02368976c174e2088851b8365d0d79a9eee709c6a05a2fad539")
	shareV := mustHex(t, "04c0f65da0d11927bdf5d560c69e1d7d939a05b0e88291887d679fcadea75810fb5cc1ca7494db39e82ff2f50665255d76173e09986ab46742c798a9a68437b048")

	context := []byte("SPAKE2+-P256-SHA256-HKDF-SHA256-HMAC-SHA256 Test Vectors")
	idProver := []byte("client")
	idVerifier := []byte("server")

	p, err := NewProver(w0, w1, x)
	if err != nil {
		t.Fatal(err)
	}
	ks, err := p.Confirm(shareV, context, idProver, idVerifier)
	if err != nil {
		t.Fatal(err)
	}

	assertHex(t, "K_main", ks.KMain, "4c59e1ccf2cfb961aa31bd9434478a1089b56cd11542f53d3576fb6c2a438a29")
	assertHex(t, "confirmP", ks.ConfirmP, "926cc713504b9b4d76c9162ded04b5493e89109f6d89462cd33adc46fda27527")
	assertHex(t, "confirmV", ks.ConfirmV, "9747bcc4f8fe9f63defee53ac9b07876d907d55047e6ff2def2e7529089d3e68")
	assertHex(t, "K_shared", ks.SharedKey, "0c5f8ccd1413423a54f6c1fb26ff01534a87f893779c6e68666d772bfd91f3e7")

	if !ks.VerifyPeer(mustHex(t, "9747bcc4f8fe9f63defee53ac9b07876d907d55047e6ff2def2e7529089d3e68")) {
		t.Fatal("VerifyPeer rejected the correct verifier MAC")
	}
	if ks.VerifyPeer(make([]byte, 32)) {
		t.Fatal("VerifyPeer accepted a wrong MAC")
	}
}
