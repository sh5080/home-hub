package cert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"
)

// VerifySignature checks the certificate's ECDSA-with-SHA256 signature over its
// DER TBSCertificate, under issuerPubKey (an uncompressed 65-byte P-256 point).
// For a self-signed root, pass the certificate's own PublicKey.
func (c *Cert) VerifySignature(issuerPubKey []byte) error {
	if len(issuerPubKey) != 65 || issuerPubKey[0] != 0x04 {
		return errors.New("cert: issuer public key must be a 65-byte uncompressed point")
	}
	if len(c.Signature) != 64 {
		return fmt.Errorf("cert: signature must be 64 bytes, got %d", len(c.Signature))
	}
	tbs, err := c.tbsDER()
	if err != nil {
		return fmt.Errorf("cert: rebuild TBS: %w", err)
	}
	hash := sha256.Sum256(tbs)

	pub := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(issuerPubKey[1:33]),
		Y:     new(big.Int).SetBytes(issuerPubKey[33:65]),
	}
	r := new(big.Int).SetBytes(c.Signature[:32])
	s := new(big.Int).SetBytes(c.Signature[32:])
	if !ecdsa.Verify(pub, hash[:], r, s) {
		return errors.New("cert: signature verification failed")
	}
	return nil
}

// VerifyChain verifies a NOC against its issuing chain: rcac must be
// self-signed, icac (if present) signed by rcac, and noc signed by whichever of
// those is its immediate issuer.
func VerifyChain(noc, icac, rcac *Cert) error {
	if err := rcac.VerifySignature(rcac.PublicKey); err != nil {
		return fmt.Errorf("rcac self-signature: %w", err)
	}
	issuer := rcac
	if icac != nil {
		if err := icac.VerifySignature(rcac.PublicKey); err != nil {
			return fmt.Errorf("icac signature: %w", err)
		}
		issuer = icac
	}
	if err := noc.VerifySignature(issuer.PublicKey); err != nil {
		return fmt.Errorf("noc signature: %w", err)
	}
	// TODO(before production): additionally enforce validity periods, the
	// issuer/subject DN linkage, basic-constraints (isCA on rcac/icac and
	// !isCA on noc), path-length, and fabric-id consistency across the chain.
	// These are intentionally omitted rather than implemented on a guess; they
	// need validation against a full CHIP chain vector. The cryptographic gate
	// (signature verification) is validated against real CHIP certs.
	return nil
}
