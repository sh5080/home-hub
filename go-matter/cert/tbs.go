package cert

import (
	"fmt"
	"time"
)

// A Matter certificate's ECDSA signature is computed over the DER-encoded X.509
// TBSCertificate, not over the TLV. tbsDER reconstructs those exact bytes so the
// signature can be verified. Every rule here is validated byte-for-byte against
// the CHIP reference certificates Root01 (RCAC) and Node01_01 (NOC).

// Fixed OIDs (DER content, without the 0x06/len wrapper).
var (
	oidECDSAWithSHA256 = []byte{0x2a, 0x86, 0x48, 0xce, 0x3d, 0x04, 0x03, 0x02}
	oidECPublicKey     = []byte{0x2a, 0x86, 0x48, 0xce, 0x3d, 0x02, 0x01}
	oidPrime256v1      = []byte{0x2a, 0x86, 0x48, 0xce, 0x3d, 0x03, 0x01, 0x07}

	oidExtBasicConstraints = []byte{0x55, 0x1d, 0x13}
	oidExtKeyUsageX        = []byte{0x55, 0x1d, 0x0f}
	oidExtExtKeyUsageX     = []byte{0x55, 0x1d, 0x25}
	oidExtSubjectKeyID     = []byte{0x55, 0x1d, 0x0e}
	oidExtAuthorityKeyID   = []byte{0x55, 0x1d, 0x23}

	// Matter DN attribute arc: 1.3.6.1.4.1.37244.1 (37244 -> 0x82 0xa2 0x7c).
	// The final arc byte is (DN tag - 16): node=1, fwsign=2, icac=3, rcac=4,
	// fabric=5, case-auth-tag=6.
	matterDNArc = []byte{0x2b, 0x06, 0x01, 0x04, 0x01, 0x82, 0xa2, 0x7c, 0x01}

	oidCommonName = []byte{0x55, 0x04, 0x03}
)

// Matter extended-key-usage purpose value -> X.509 id-kp-* OID (1.3.6.1.5.5.7.3.N).
var ekuOIDs = map[uint8][]byte{
	1: {0x2b, 0x06, 0x01, 0x05, 0x05, 0x07, 0x03, 0x01}, // serverAuth
	2: {0x2b, 0x06, 0x01, 0x05, 0x05, 0x07, 0x03, 0x02}, // clientAuth
	3: {0x2b, 0x06, 0x01, 0x05, 0x05, 0x07, 0x03, 0x03}, // codeSigning
	4: {0x2b, 0x06, 0x01, 0x05, 0x05, 0x07, 0x03, 0x09}, // ocspSigning
}

const matterEpochToUnix = 946684800 // 2000-01-01T00:00:00Z as a Unix timestamp

// tbsDER reconstructs the DER TBSCertificate.
func (c *Cert) tbsDER() ([]byte, error) {
	version := derTLV(0xa0, derInt([]byte{0x02})) // [0] EXPLICIT INTEGER 2 (v3)
	serial := derInt(c.SerialNumber)
	sigAlgo := derSeq(derOID(oidECDSAWithSHA256))

	issuer, err := dnDER(c.Issuer)
	if err != nil {
		return nil, fmt.Errorf("issuer: %w", err)
	}
	nb, err := matterTimeDER(c.NotBefore)
	if err != nil {
		return nil, err
	}
	na, err := matterTimeDER(c.NotAfter)
	if err != nil {
		return nil, err
	}
	validity := derSeq(nb, na)

	subject, err := dnDER(c.Subject)
	if err != nil {
		return nil, fmt.Errorf("subject: %w", err)
	}

	spki := derSeq(
		derSeq(derOID(oidECPublicKey), derOID(oidPrime256v1)),
		derBitString(0x00, c.PublicKey),
	)

	exts, err := extensionsDER(c.Extensions)
	if err != nil {
		return nil, err
	}
	extBlock := derTLV(0xa3, derSeq(exts)) // [3] EXPLICIT SEQUENCE OF Extension

	return derSeq(version, serial, sigAlgo, issuer, validity, subject, spki, extBlock), nil
}

// dnDER encodes a distinguished name as an X.509 RDNSequence.
func dnDER(dn DN) ([]byte, error) {
	var rdns []byte
	for _, a := range dn.Attrs {
		var oid, val []byte
		switch {
		case a.Tag >= DNMatterNodeID && a.Tag <= DNMatterCASEAuthTag:
			// Matter id attributes render as an uppercase fixed-width hex string:
			// 16 digits for 64-bit ids, 8 digits for the 32-bit CASE auth tag.
			arc := a.Tag - 16
			oid = append(append([]byte{}, matterDNArc...), arc)
			width := 16
			if a.Tag == DNMatterCASEAuthTag {
				width = 8
			}
			val = derUTF8(fmt.Sprintf("%0*X", width, a.Value))
		case a.Tag == DNCommonName:
			oid = oidCommonName
			if a.Printable {
				val = derPrintable(a.String)
			} else {
				val = derUTF8(a.String)
			}
		default:
			// VERIFY: other standard DN attributes (surname, org, ...) are not
			// exercised by the Matter operational certs we handle; add their
			// OIDs here if a real cert ever needs them.
			return nil, fmt.Errorf("cert: DN tag %d unsupported for DER", a.Tag)
		}
		rdns = append(rdns, derSet(derSeq(derOID(oid), val))...)
	}
	return derSeq(rdns), nil
}

// matterTimeDER converts Matter epoch seconds to an X.509 time. Per CHIP, the
// sentinel value 0 means "no well-defined expiry" (GeneralizedTime 9999...).
// Years in [1950,2049] use UTCTime; later years use GeneralizedTime.
func matterTimeDER(sec uint32) ([]byte, error) {
	if sec == 0 {
		return derTLV(0x18, []byte("99991231235959Z")), nil
	}
	t := time.Unix(int64(sec)+matterEpochToUnix, 0).UTC()
	if y := t.Year(); y >= 1950 && y <= 2049 {
		return derTLV(0x17, []byte(t.Format("060102150405Z"))), nil
	}
	return derTLV(0x18, []byte(t.Format("20060102150405Z"))), nil
}

func extensionsDER(ext Extensions) ([]byte, error) {
	var out []byte
	add := func(b []byte) { out = append(out, b...) }

	if bc := ext.BasicConstraints; bc != nil {
		var inner []byte
		// DER omits the cA BOOLEAN when false (its default), and only then.
		if bc.IsCA {
			inner = append(inner, derBool(true)...)
		}
		if bc.PathLen != nil {
			inner = append(inner, derInt([]byte{*bc.PathLen})...)
		}
		add(derSeq(derOID(oidExtBasicConstraints), derBool(true), derOctet(derSeq(inner))))
	}
	if ext.KeyUsage != nil {
		add(derSeq(derOID(oidExtKeyUsageX), derBool(true), derOctet(keyUsageBitString(*ext.KeyUsage))))
	}
	if ext.ExtKeyUsage != nil {
		var oids []byte
		for _, v := range ext.ExtKeyUsage {
			oid, ok := ekuOIDs[v]
			if !ok {
				return nil, fmt.Errorf("cert: unknown extended-key-usage purpose %d", v)
			}
			oids = append(oids, derOID(oid)...)
		}
		add(derSeq(derOID(oidExtExtKeyUsageX), derBool(true), derOctet(derSeq(oids))))
	}
	if ext.SubjectKeyID != nil {
		add(derSeq(derOID(oidExtSubjectKeyID), derOctet(derOctet(ext.SubjectKeyID))))
	}
	if ext.AuthorityKeyID != nil {
		// AuthorityKeyIdentifier ::= SEQUENCE { keyIdentifier [0] IMPLICIT OCTET STRING }
		akid := derSeq(derTLV(0x80, ext.AuthorityKeyID))
		add(derSeq(derOID(oidExtAuthorityKeyID), derOctet(akid)))
	}
	return out, nil
}

// keyUsageBitString renders the Matter key-usage bitmap as an X.509 KeyUsage
// BIT STRING. Bit N of the Matter bitmap maps to X.509 KeyUsage bit N, which is
// stored MSB-first (bit N at byte N/8, mask 0x80>>(N%8)); trailing unused bits
// are counted in the leading octet.
func keyUsageBitString(bitmap uint16) []byte {
	var content []byte
	highest := -1
	for n := 0; n < 16; n++ {
		if bitmap&(1<<n) == 0 {
			continue
		}
		byteIdx := n / 8
		for len(content) <= byteIdx {
			content = append(content, 0)
		}
		content[byteIdx] |= 0x80 >> (n % 8)
		highest = n
	}
	if highest < 0 {
		return derBitString(0, nil)
	}
	used := highest + 1
	unused := byte((8 - used%8) % 8)
	return derBitString(unused, content[:(used+7)/8])
}
