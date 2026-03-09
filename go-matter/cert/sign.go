package cert

import "github.com/sh5080/go-matter/crypto"

// SignAndEncode signs the certificate's TBSCertificate with the issuer's
// operational private key (a 32-byte P-256 scalar) and returns the complete
// Matter TLV certificate. A self-signed root passes its own key. This is the
// issuance path a controller-CA uses during commissioning.
func (c *Cert) SignAndEncode(issuerOpKey []byte) ([]byte, error) {
	tbs, err := c.tbsDER()
	if err != nil {
		return nil, err
	}
	sig, err := crypto.SignECDSA(issuerOpKey, tbs)
	if err != nil {
		return nil, err
	}
	c.Signature = sig
	return c.Encode()
}
