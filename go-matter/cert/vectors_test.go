package cert

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// Real Matter test certificate "Root01" from CHIP
// (src/credentials/tests/CHIPCert_test_vectors.cpp): a self-signed RCAC, in both
// its Matter TLV form and its X.509 DER form, plus the subject public key. Used
// to validate the codec and the TLV->DER conversion against a conforming impl.
const (
	root01ChipHex = "15300108534c4582736235142402013703271401000000cacacaca182604ef171b2726056eb5b94c3706271401000000cacacaca18240701240801300941043b88460ec9687a5d0f3b4b3b13fcd299c2f6d5051d003ee49c9924cf98f4f780eb20fd37c8d358347f5f87d08c3213e540af11bab9137e49354f0c5b6343de63370a3501290118240260300414cc1308af82cfee505eb23b57bfe86a311665535f300514cc1308af82cfee505eb23b57bfe86a311665535f18300b40f7f0092690494e46c8b1c5cbd1a5085e1e65d4360f98e96c4e8e495dc5e216d0bfa23d8f57470d89fddaf03f0464b0ae8e1f956d6f67a31124385824689780a918"

	root01DERHex = "3082019e30820143a0030201020208534c458273623514300a06082a8648ce3d04030230223120301e060a2b0601040182a27c01040c1043414341434143413030303030303031301e170d3230313031353134323334335a170d3430313031353134323334325a30223120301e060a2b0601040182a27c01040c10434143414341434130303030303030313059301306072a8648ce3d020106082a8648ce3d030107034200043b88460ec9687a5d0f3b4b3b13fcd299c2f6d5051d003ee49c9924cf98f4f780eb20fd37c8d358347f5f87d08c3213e540af11bab9137e49354f0c5b6343de63a3633061300f0603551d130101ff040530030101ff300e0603551d0f0101ff040403020106301d0603551d0e04160414cc1308af82cfee505eb23b57bfe86a311665535f301f0603551d23041830168014cc1308af82cfee505eb23b57bfe86a311665535f300a06082a8648ce3d0403020349003046022100f7f0092690494e46c8b1c5cbd1a5085e1e65d4360f98e96c4e8e495dc5e216d0022100bfa23d8f57470d89fddaf03f0464b0ae8e1f956d6f67a31124385824689780a9"

	root01PubKeyHex = "043b88460ec9687a5d0f3b4b3b13fcd299c2f6d5051d003ee49c9924cf98f4f780eb20fd37c8d358347f5f87d08c3213e540af11bab9137e49354f0c5b6343de63"
)

func decodeHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestDecodeRealRoot01 decodes the real CHIP RCAC and re-encodes it, requiring
// byte-exact reproduction — a strong check that the TLV codec matches a
// conforming implementation on real data.
func TestDecodeRealRoot01(t *testing.T) {
	orig := decodeHex(t, root01ChipHex)
	c, err := Decode(orig)
	if err != nil {
		t.Fatalf("decode Root01: %v", err)
	}

	if rid, ok := c.Subject.uintAttr(DNMatterRCACID); !ok || rid != 0xcacacaca00000001 {
		t.Fatalf("subject rcac-id = %#x (ok=%v)", rid, ok)
	}
	if !bytes.Equal(c.PublicKey, decodeHex(t, root01PubKeyHex)) {
		t.Fatalf("public key mismatch: %x", c.PublicKey)
	}
	if c.Extensions.BasicConstraints == nil || !c.Extensions.BasicConstraints.IsCA {
		t.Fatal("Root01 should be a CA")
	}
	if len(c.Signature) != 64 {
		t.Fatalf("signature length %d", len(c.Signature))
	}

	reenc, err := c.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(orig, reenc) {
		t.Fatalf("re-encode not byte-exact against real cert:\n orig=%x\n got =%x", orig, reenc)
	}
}
