package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"

	"filippo.io/nistec"
)

// SignECDSA produces a raw 64-byte r||s ECDSA-with-SHA256 signature over message
// under a P-256 private key given as a 32-byte big-endian scalar. Matter uses
// this fixed-width signature format throughout (Spec 3.5.3).
func SignECDSA(privScalar, message []byte) ([]byte, error) {
	if len(privScalar) != 32 {
		return nil, fmt.Errorf("crypto: private scalar must be 32 bytes, got %d", len(privScalar))
	}
	pt, err := nistec.NewP256Point().ScalarBaseMult(privScalar)
	if err != nil {
		return nil, fmt.Errorf("crypto: derive public key: %w", err)
	}
	pub := pt.Bytes() // uncompressed 65-byte point
	priv := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: elliptic.P256(),
			X:     new(big.Int).SetBytes(pub[1:33]),
			Y:     new(big.Int).SetBytes(pub[33:65]),
		},
		D: new(big.Int).SetBytes(privScalar),
	}
	hash := sha256.Sum256(message)
	r, s, err := ecdsa.Sign(rand.Reader, priv, hash[:])
	if err != nil {
		return nil, err
	}
	out := make([]byte, 64)
	r.FillBytes(out[:32])
	s.FillBytes(out[32:])
	return out, nil
}

// VerifyECDSA verifies a raw 64-byte r||s signature over message under an
// uncompressed 65-byte P-256 public key.
func VerifyECDSA(pubKey, message, sig []byte) error {
	if len(pubKey) != 65 || pubKey[0] != 0x04 {
		return errors.New("crypto: public key must be a 65-byte uncompressed point")
	}
	if len(sig) != 64 {
		return errors.New("crypto: signature must be 64 bytes")
	}
	pub := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(pubKey[1:33]),
		Y:     new(big.Int).SetBytes(pubKey[33:65]),
	}
	hash := sha256.Sum256(message)
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(pub, hash[:], r, s) {
		return errors.New("crypto: ECDSA verification failed")
	}
	return nil
}

// PublicFromScalar returns the uncompressed 65-byte P-256 public key for a
// 32-byte private scalar.
func PublicFromScalar(privScalar []byte) ([]byte, error) {
	if len(privScalar) != 32 {
		return nil, fmt.Errorf("crypto: private scalar must be 32 bytes, got %d", len(privScalar))
	}
	pt, err := nistec.NewP256Point().ScalarBaseMult(privScalar)
	if err != nil {
		return nil, err
	}
	return pt.Bytes(), nil
}
