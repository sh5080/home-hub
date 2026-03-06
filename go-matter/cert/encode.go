package cert

import "github.com/sh5080/go-matter/tlv"

// Encode serializes the certificate to its Matter TLV form. Encoding the result
// of Decode reproduces the original bytes for certificates using the modeled
// fields (minimal integer widths, as produced by conforming encoders).
func (c *Cert) Encode() ([]byte, error) {
	w := tlv.NewWriter()
	w.StartStructure(tlv.Anonymous())
	w.PutBytes(tlv.Context(tagSerialNumber), c.SerialNumber)
	w.PutUint(tlv.Context(tagSigAlgo), c.SigAlgo)
	encodeDN(w, tagIssuer, c.Issuer)
	w.PutUint(tlv.Context(tagNotBefore), uint64(c.NotBefore))
	w.PutUint(tlv.Context(tagNotAfter), uint64(c.NotAfter))
	encodeDN(w, tagSubject, c.Subject)
	w.PutUint(tlv.Context(tagPubKeyAlgo), c.PubKeyAlgo)
	w.PutUint(tlv.Context(tagCurveID), c.CurveID)
	w.PutBytes(tlv.Context(tagPubKey), c.PublicKey)
	encodeExtensions(w, c.Extensions)
	w.PutBytes(tlv.Context(tagSignature), c.Signature)
	w.EndContainer()
	return w.Bytes()
}

func encodeDN(w *tlv.Writer, listTag uint8, dn DN) {
	w.StartList(tlv.Context(listTag))
	for _, a := range dn.Attrs {
		t := a.Tag
		if a.IsString {
			if a.Printable {
				t |= printableFlag
			}
			w.PutString(tlv.Context(t), a.String)
		} else {
			w.PutUint(tlv.Context(t), a.Value)
		}
	}
	w.EndContainer()
}

func encodeExtensions(w *tlv.Writer, ext Extensions) {
	w.StartList(tlv.Context(tagExtensions))
	if bc := ext.BasicConstraints; bc != nil {
		w.StartStructure(tlv.Context(extBasicConstraints))
		w.PutBool(tlv.Context(bcIsCA), bc.IsCA)
		if bc.PathLen != nil {
			w.PutUint(tlv.Context(bcPathLen), uint64(*bc.PathLen))
		}
		w.EndContainer()
	}
	if ext.KeyUsage != nil {
		w.PutUint(tlv.Context(extKeyUsage), uint64(*ext.KeyUsage))
	}
	if ext.ExtKeyUsage != nil {
		w.StartArray(tlv.Context(extExtendedKeyUsage))
		for _, v := range ext.ExtKeyUsage {
			w.PutUint(tlv.Anonymous(), uint64(v))
		}
		w.EndContainer()
	}
	if ext.SubjectKeyID != nil {
		w.PutBytes(tlv.Context(extSubjectKeyID), ext.SubjectKeyID)
	}
	if ext.AuthorityKeyID != nil {
		w.PutBytes(tlv.Context(extAuthorityKeyID), ext.AuthorityKeyID)
	}
	w.EndContainer()
}
