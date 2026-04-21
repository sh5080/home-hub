package discovery

import (
	"encoding/hex"
	"testing"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The root public key from the CASE spec test vector; reused here to exercise
// the compressed-fabric-id derivation against a realistic key.
const specRootPubKey = "044a9f42b1ca4840d37292bbc7f6a7e11e22200c976fc900dbc98a7a383a641cb8254a2e56d4e295a847943b4e3897c4a773e930277b4d9fbede8a052686bfacfa"

func TestCompressedFabricID(t *testing.T) {
	root := mustHex(t, specRootPubKey)

	id, err := CompressedFabricID(root, 0x2906C908D115D362)
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 8 {
		t.Fatalf("compressed fabric id length %d, want 8", len(id))
	}

	// Deterministic.
	id2, _ := CompressedFabricID(root, 0x2906C908D115D362)
	if hex.EncodeToString(id) != hex.EncodeToString(id2) {
		t.Fatal("compressed fabric id is not deterministic")
	}
	// Depends on the fabric id.
	other, _ := CompressedFabricID(root, 0x2906C908D115D363)
	if hex.EncodeToString(id) == hex.EncodeToString(other) {
		t.Fatal("compressed fabric id did not depend on fabric id")
	}
	// Rejects a malformed key.
	if _, err := CompressedFabricID(root[:64], 1); err == nil {
		t.Fatal("expected error for short root key")
	}
}

func TestOperationalInstanceName(t *testing.T) {
	cfid := mustHex(t, "2906c908d115d362")
	got := OperationalInstanceName(cfid, 0xCD5544AA7B13EF14)
	want := "2906C908D115D362-CD5544AA7B13EF14"
	if got != want {
		t.Fatalf("instance name = %q, want %q", got, want)
	}
}
