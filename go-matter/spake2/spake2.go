// Package spake2 implements the prover (commissioner) side of SPAKE2+ over
// P256-SHA256, as used by Matter PASE (Matter Spec 3.10, RFC 9383). The prover
// is the commissioner and the verifier is the device being commissioned.
//
// Point arithmetic uses filippo.io/nistec (constant-time P-256). This file
// covers the group math (shares and the shared points Z, V); the Matter
// transcript and confirmation-key schedule are built on top of it.
package spake2

import (
	"crypto/elliptic"
	"encoding/hex"
	"fmt"
	"math/big"

	"filippo.io/nistec"
)

// Compressed M and N points for P-256 (RFC 9383 §4 / Matter Spec 3.10).
const (
	mHex = "02886e2f97ace46e55ba9dd7242579f2993b64e16ef3dcab95afd497333d8fa12f"
	nHex = "03d8bbd6c639c62937b04d997f38c3770719c629d7014d49a24b4f98baa1292b49"
)

var (
	pointM = mustPoint(mHex)
	pointN = mustPoint(nHex)
	order  = elliptic.P256().Params().N // group order n
)

func mustPoint(h string) *nistec.P256Point {
	b, err := hex.DecodeString(h)
	if err != nil {
		panic("spake2: bad point hex: " + err.Error())
	}
	p, err := nistec.NewP256Point().SetBytes(b)
	if err != nil {
		panic("spake2: cannot decode point: " + err.Error())
	}
	return p
}

// scalarBytes reduces k modulo the group order and returns 32 big-endian bytes.
func scalarBytes(k *big.Int) []byte {
	b := make([]byte, 32)
	new(big.Int).Mod(k, order).FillBytes(b)
	return b
}

// Prover holds the commissioner's SPAKE2+ state.
type Prover struct {
	w0, w1 *big.Int
	x      *big.Int
	share  *nistec.P256Point // X = x*G + w0*M
}

// NewProver creates a prover from the registration scalars w0, w1 and the
// ephemeral secret x. x is a parameter (rather than generated internally) so
// callers can inject a fixed value for known-answer tests; production callers
// pass a fresh random scalar in [1, n-1].
func NewProver(w0, w1, x *big.Int) (*Prover, error) {
	xG, err := nistec.NewP256Point().ScalarBaseMult(scalarBytes(x))
	if err != nil {
		return nil, fmt.Errorf("spake2: x*G: %w", err)
	}
	w0M, err := nistec.NewP256Point().ScalarMult(pointM, scalarBytes(w0))
	if err != nil {
		return nil, fmt.Errorf("spake2: w0*M: %w", err)
	}
	return &Prover{
		w0:    w0,
		w1:    w1,
		x:     x,
		share: nistec.NewP256Point().Add(xG, w0M),
	}, nil
}

// Share returns the prover's public share pA = X (uncompressed SEC1, 65 bytes).
func (p *Prover) Share() []byte { return p.share.Bytes() }

// Finish consumes the verifier's share pB = Y and returns the shared secret
// points Z and V (uncompressed), the inputs to the transcript and confirmation
// keys. It fails if pB is not a valid curve point.
func (p *Prover) Finish(pB []byte) (z, v []byte, err error) {
	y, err := nistec.NewP256Point().SetBytes(pB)
	if err != nil {
		return nil, nil, fmt.Errorf("spake2: invalid verifier share: %w", err)
	}
	// T = Y - w0*N
	w0N, err := nistec.NewP256Point().ScalarMult(pointN, scalarBytes(p.w0))
	if err != nil {
		return nil, nil, fmt.Errorf("spake2: w0*N: %w", err)
	}
	t := nistec.NewP256Point().Add(y, nistec.NewP256Point().Negate(w0N))
	// Z = x*T, V = w1*T
	zp, err := nistec.NewP256Point().ScalarMult(t, scalarBytes(p.x))
	if err != nil {
		return nil, nil, fmt.Errorf("spake2: x*T: %w", err)
	}
	vp, err := nistec.NewP256Point().ScalarMult(t, scalarBytes(p.w1))
	if err != nil {
		return nil, nil, fmt.Errorf("spake2: w1*T: %w", err)
	}
	return zp.Bytes(), vp.Bytes(), nil
}
