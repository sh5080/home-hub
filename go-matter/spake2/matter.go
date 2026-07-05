package spake2

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/big"

	"filippo.io/nistec"

	"github.com/sh5080/go-matter/crypto"
)

// Matter's PASE uses the SPAKE2+ draft-02 key schedule (Spec 3.10 / CHIP
// Spake2p::GenerateKeys), which differs from RFC 9383's final one:
//
//	Ka || Ke   = Hash(TT)                              (16 bytes each)
//	KcA || KcB = HKDF(Ka, salt=nil, "ConfirmationKeys") (16 bytes each)
//	cA = HMAC(KcA, pB),  cB = HMAC(KcB, pA)
//
// The transcript TT itself is identical to RFC 9383 §3.3.

// MatterKeys is the draft-02 key schedule output used by PASE.
type MatterKeys struct {
	Ke []byte // 16-byte shared key, input to the session-key HKDF
	CA []byte // expected prover confirmation cA = HMAC(KcA, shareV)
	CB []byte // expected verifier confirmation cB = HMAC(KcB, shareP)
}

// deriveMatterKeys computes the draft-02 schedule from a completed exchange.
func deriveMatterKeys(context, idProver, idVerifier, shareP, shareV, z, v, w0 []byte) (*MatterKeys, error) {
	tt := transcript(context, idProver, idVerifier, shareP, shareV, z, v, w0)
	kae := sha256.Sum256(tt)
	ka, ke := kae[:16], kae[16:]

	kcab, err := crypto.HKDF(ka, nil, []byte("ConfirmationKeys"), 32)
	if err != nil {
		return nil, err
	}
	kca, kcb := kcab[:16], kcab[16:]

	return &MatterKeys{
		Ke: ke,
		CA: hmacSHA256(kca, shareV),
		CB: hmacSHA256(kcb, shareP),
	}, nil
}

// ConfirmMatter is the prover-side completion under the Matter key schedule.
// PASE uses empty prover/verifier identities.
func (p *Prover) ConfirmMatter(pB, context []byte) (*MatterKeys, error) {
	z, v, err := p.Finish(pB)
	if err != nil {
		return nil, err
	}
	return deriveMatterKeys(context, nil, nil, p.Share(), pB, z, v, scalarBytes(p.w0))
}

// VerifyCB checks the verifier's confirmation MAC in constant time.
func (k *MatterKeys) VerifyCB(cB []byte) bool { return hmac.Equal(k.CB, cB) }

// VerifyCA checks the prover's confirmation MAC in constant time.
func (k *MatterKeys) VerifyCA(cA []byte) bool { return hmac.Equal(k.CA, cA) }

// wsLength is the per-scalar PBKDF2 output length (Spec 3.10: FE length + 8,
// oversized so the mod-n reduction is unbiased).
const wsLength = 40

// DeriveW derives the SPAKE2+ registration scalars from a setup passcode
// (CHIP Spake2pVerifier::ComputeWS): PBKDF2-SHA256 over the passcode encoded
// as 4 little-endian bytes, producing w0s||w1s (40 bytes each), each reduced
// mod the group order.
func DeriveW(passcode uint32, salt []byte, iterations int) (w0, w1 *big.Int, err error) {
	if len(salt) < 16 || len(salt) > 32 {
		return nil, nil, fmt.Errorf("spake2: salt length %d out of range [16,32]", len(salt))
	}
	if iterations < 1000 || iterations > 100000 {
		return nil, nil, fmt.Errorf("spake2: iteration count %d out of range [1000,100000]", iterations)
	}
	var pin [4]byte
	binary.LittleEndian.PutUint32(pin[:], passcode)
	ws, err := crypto.PBKDF2(pin[:], salt, iterations, 2*wsLength)
	if err != nil {
		return nil, nil, err
	}
	w0 = new(big.Int).Mod(new(big.Int).SetBytes(ws[:wsLength]), order)
	w1 = new(big.Int).Mod(new(big.Int).SetBytes(ws[wsLength:]), order)
	return w0, w1, nil
}

// ComputeL returns L = w1*G (uncompressed), the verifier's registration record.
func ComputeL(w1 *big.Int) ([]byte, error) {
	l, err := nistec.NewP256Point().ScalarBaseMult(scalarBytes(w1))
	if err != nil {
		return nil, fmt.Errorf("spake2: w1*G: %w", err)
	}
	return l.Bytes(), nil
}

// Verifier holds the device-side SPAKE2+ state. A commissionee knows w0 and
// L = w1*G (its registration record), but not w1 itself.
type Verifier struct {
	w0    *big.Int
	l     *nistec.P256Point
	y     *big.Int
	share *nistec.P256Point // Y = y*G + w0*N
}

// NewVerifier creates the device side from its registration record. y is the
// ephemeral secret (parameterized for known-answer tests; production passes a
// fresh random scalar).
func NewVerifier(w0 *big.Int, l []byte, y *big.Int) (*Verifier, error) {
	lp, err := nistec.NewP256Point().SetBytes(l)
	if err != nil {
		return nil, fmt.Errorf("spake2: invalid L: %w", err)
	}
	yG, err := nistec.NewP256Point().ScalarBaseMult(scalarBytes(y))
	if err != nil {
		return nil, fmt.Errorf("spake2: y*G: %w", err)
	}
	w0N, err := nistec.NewP256Point().ScalarMult(pointN, scalarBytes(w0))
	if err != nil {
		return nil, fmt.Errorf("spake2: w0*N: %w", err)
	}
	return &Verifier{
		w0:    w0,
		l:     lp,
		y:     y,
		share: nistec.NewP256Point().Add(yG, w0N),
	}, nil
}

// Share returns the verifier's public share pB = Y (uncompressed, 65 bytes).
func (v *Verifier) Share() []byte { return v.share.Bytes() }

// ConfirmMatter consumes the prover's share pA and derives the Matter key
// schedule: Z = y*(X - w0*M), V = y*L.
func (v *Verifier) ConfirmMatter(pA, context []byte) (*MatterKeys, error) {
	x, err := nistec.NewP256Point().SetBytes(pA)
	if err != nil {
		return nil, fmt.Errorf("spake2: invalid prover share: %w", err)
	}
	w0M, err := nistec.NewP256Point().ScalarMult(pointM, scalarBytes(v.w0))
	if err != nil {
		return nil, fmt.Errorf("spake2: w0*M: %w", err)
	}
	t := nistec.NewP256Point().Add(x, nistec.NewP256Point().Negate(w0M))
	z, err := nistec.NewP256Point().ScalarMult(t, scalarBytes(v.y))
	if err != nil {
		return nil, fmt.Errorf("spake2: y*T: %w", err)
	}
	vv, err := nistec.NewP256Point().ScalarMult(v.l, scalarBytes(v.y))
	if err != nil {
		return nil, fmt.Errorf("spake2: y*L: %w", err)
	}
	return deriveMatterKeys(context, nil, nil, pA, v.Share(), z.Bytes(), vv.Bytes(), scalarBytes(v.w0))
}
