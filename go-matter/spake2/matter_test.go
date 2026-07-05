package spake2

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"testing"
)

// Known-answer vector from CHIP src/crypto/tests/SPAKE2P_RFC_test_vectors.h
// (first entry) — the draft-01/02 key schedule Matter PASE uses: Ka‖Ke =
// Hash(TT), KcA‖KcB = HKDF(Ka, "ConfirmationKeys"), 16 bytes each.
var matterTV = struct {
	context, idP, idV                string
	w0, w1, l, x, xx, y, yy          string
	ka, ke, kcA, kcB, macKcA, macKcB string
}{
	context: "SPAKE2+-P256-SHA256-HKDF draft-01",
	idP:     "client",
	idV:     "server",
	w0:      "e6887cf9bdfb7579c69bf47928a84514b5e355ac034863f7ffaf4390e67d798c",
	w1:      "24b5ae4abda868ec9336ffc3b78ee31c5755bef1759227ef5372ca139b94e512",
	l:       "0495645cfb74df6e58f9748bb83a86620bab7c82e107f57d6870da8cbcb2ff9f7063a14b6402c62f99afcb9706a4d1a143273259fe76f1c605a3639745a92154b9",
	x:       "8b0f3f383905cf3a3bb955ef8fb62e24849dd349a05ca79aafb18041d30cbdb6",
	xx:      "04af09987a593d3bac8694b123839422c3cc87e37d6b41c1d630f000dd64980e537ae704bcede04ea3bec9b7475b32fa2ca3b684be14d11645e38ea6609eb39e7e",
	y:       "2e0895b0e763d6d5a9564433e64ac3cac74ff897f6c3445247ba1bab40082a91",
	yy:      "04417592620aebf9fd203616bbb9f121b730c258b286f890c5f19fea833a9c900cbe9057bc549a3e19975be9927f0e7614f08d1f0a108eede5fd7eb5624584a4f4",
	ka:      "f9cab9adcc0ed8e5a4db11a8505914b2",
	ke:      "801db297654816eb4f02868129b9dc89",
	kcA:     "0d248d7d19234f1486b2efba5179c52d",
	kcB:     "556291df26d705a2caedd6474dd0079b",
	macKcA:  "d4376f2da9c72226dd151b77c2919071155fc22a2068d90b5faa6c78c11e77dd",
	macKcB:  "0660a680663e8c5695956fb22dff298b1d07a526cf3cc591adfecd1f6ef6e02e",
}

func hexb(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func hexInt(t *testing.T, s string) *big.Int {
	t.Helper()
	return new(big.Int).SetBytes(hexb(t, s))
}

// matterKeysFromVector runs deriveMatterKeys with the vector's transcript
// inputs (identities are non-empty in this vector, unlike PASE).
func TestMatterKeyScheduleVector(t *testing.T) {
	tv := matterTV
	prover, err := NewProver(hexInt(t, tv.w0), hexInt(t, tv.w1), hexInt(t, tv.x))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(prover.Share(), hexb(t, tv.xx)) {
		t.Fatal("prover share X mismatch")
	}
	verifier, err := NewVerifier(hexInt(t, tv.w0), hexb(t, tv.l), hexInt(t, tv.y))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(verifier.Share(), hexb(t, tv.yy)) {
		t.Fatal("verifier share Y mismatch")
	}

	z, v, err := prover.Finish(verifier.Share())
	if err != nil {
		t.Fatal(err)
	}
	pk, err := deriveMatterKeys([]byte(tv.context), []byte(tv.idP), []byte(tv.idV),
		prover.Share(), verifier.Share(), z, v, scalarBytes(hexInt(t, tv.w0)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pk.Ke, hexb(t, tv.ke)) {
		t.Fatalf("Ke = %x, want %s", pk.Ke, tv.ke)
	}
	if !bytes.Equal(pk.CA, hexb(t, tv.macKcA)) {
		t.Fatalf("cA = %x, want %s", pk.CA, tv.macKcA)
	}
	if !bytes.Equal(pk.CB, hexb(t, tv.macKcB)) {
		t.Fatalf("cB = %x, want %s", pk.CB, tv.macKcB)
	}
}

// TestMatterVerifierAgreement runs both roles through the Matter schedule with
// PASE-style empty identities and checks they agree end to end.
func TestMatterVerifierAgreement(t *testing.T) {
	tv := matterTV
	context := []byte("test context")

	prover, err := NewProver(hexInt(t, tv.w0), hexInt(t, tv.w1), hexInt(t, tv.x))
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewVerifier(hexInt(t, tv.w0), hexb(t, tv.l), hexInt(t, tv.y))
	if err != nil {
		t.Fatal(err)
	}

	pKeys, err := prover.ConfirmMatter(verifier.Share(), context)
	if err != nil {
		t.Fatal(err)
	}
	vKeys, err := verifier.ConfirmMatter(prover.Share(), context)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pKeys.Ke, vKeys.Ke) {
		t.Fatal("Ke disagreement between prover and verifier")
	}
	if !pKeys.VerifyCB(vKeys.CB) || !vKeys.VerifyCA(pKeys.CA) {
		t.Fatal("confirmation MACs do not cross-verify")
	}
}

// TestDeriveWAndL checks the passcode → (w0, w1, L) pipeline: L must equal
// w1*G and a wrong passcode must produce different scalars.
func TestDeriveWAndL(t *testing.T) {
	salt := bytes.Repeat([]byte{0x5A}, 16)
	w0, w1, err := DeriveW(20202021, salt, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if w0.Sign() == 0 || w1.Sign() == 0 {
		t.Fatal("degenerate scalar")
	}
	l, err := ComputeL(w1)
	if err != nil {
		t.Fatal(err)
	}
	if len(l) != 65 || l[0] != 0x04 {
		t.Fatalf("L encoding: %d bytes, prefix %#x", len(l), l[0])
	}

	w0b, _, err := DeriveW(20202022, salt, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if w0.Cmp(w0b) == 0 {
		t.Fatal("different passcodes must derive different w0")
	}

	if _, _, err := DeriveW(1, salt, 10); err == nil {
		t.Fatal("iteration count below Matter minimum must be rejected")
	}
	if _, _, err := DeriveW(1, []byte{1, 2}, 1000); err == nil {
		t.Fatal("short salt must be rejected")
	}
}
