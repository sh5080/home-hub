// Package cert encodes and decodes Matter operational certificates in their
// TLV form (Matter Spec 6.5). Tag numbers are taken from the specification and
// cross-checked against CHIP (src/credentials/CHIPCert.h and the ASN.1 OID
// low-byte encoding): top-level fields 1-11, extensions 1-6, and DN attribute
// tags where the matter-specific attributes are 17..22.
package cert

import (
	"fmt"

	"github.com/sh5080/go-matter/tlv"
)

// Top-level certificate element tags (Spec 6.5.2 / CHIPCert.h).
const (
	tagSerialNumber = 1
	tagSigAlgo      = 2
	tagIssuer       = 3
	tagNotBefore    = 4
	tagNotAfter     = 5
	tagSubject      = 6
	tagPubKeyAlgo   = 7
	tagCurveID      = 8
	tagPubKey       = 9
	tagExtensions   = 10
	tagSignature    = 11
)

// DN attribute base tags (Spec 6.5.6.1; CHIP OID low byte). A PrintableString
// string attribute has printableFlag OR-ed into its tag.
const (
	DNCommonName        = 1
	DNMatterNodeID      = 17
	DNMatterFwSigningID = 18
	DNMatterICACID      = 19
	DNMatterRCACID      = 20
	DNMatterFabricID    = 21
	DNMatterCASEAuthTag = 22
	printableFlag       = 0x80
)

// Extension tags (Spec 6.5.11 / CHIPCert.h) and BasicConstraints sub-tags.
const (
	extBasicConstraints = 1
	extKeyUsage         = 2
	extExtendedKeyUsage = 3
	extSubjectKeyID     = 4
	extAuthorityKeyID   = 5
	extFutureExtension  = 6

	bcIsCA    = 1
	bcPathLen = 2
)

// Attr is one distinguished-name attribute.
type Attr struct {
	Tag       uint8  // base attribute tag (printable flag stripped)
	IsString  bool   // true: String is set; false: Value is set
	Printable bool   // PrintableString (vs UTF8String) for string attributes
	String    string
	Value     uint64
}

// DN is a Matter distinguished name (an ordered set of attributes).
type DN struct{ Attrs []Attr }

func (d DN) uintAttr(tag uint8) (uint64, bool) {
	for _, a := range d.Attrs {
		if a.Tag == tag && !a.IsString {
			return a.Value, true
		}
	}
	return 0, false
}

// NodeID returns the matter-node-id attribute, if present.
func (d DN) NodeID() (uint64, bool) { return d.uintAttr(DNMatterNodeID) }

// FabricID returns the matter-fabric-id attribute, if present.
func (d DN) FabricID() (uint64, bool) { return d.uintAttr(DNMatterFabricID) }

// BasicConstraints models the certificate's basic-constraints extension.
type BasicConstraints struct {
	IsCA    bool
	PathLen *uint8
}

// Extensions holds the subset of certificate extensions Matter uses.
type Extensions struct {
	BasicConstraints *BasicConstraints
	KeyUsage         *uint16
	ExtKeyUsage      []uint8
	SubjectKeyID     []byte
	AuthorityKeyID   []byte
}

// Cert is a decoded Matter operational certificate.
type Cert struct {
	SerialNumber []byte
	SigAlgo      uint64
	Issuer       DN
	NotBefore    uint32 // seconds since the Matter epoch (2000-01-01 UTC)
	NotAfter     uint32
	Subject      DN
	PubKeyAlgo   uint64
	CurveID      uint64
	PublicKey    []byte // uncompressed EC point (65 bytes)
	Extensions   Extensions
	Signature    []byte // raw r||s (64 bytes)
}

// Decode parses a Matter certificate from its TLV encoding.
func Decode(data []byte) (*Cert, error) {
	r := tlv.NewReader(data)
	if !r.Next() || r.Type() != tlv.TypeStructure {
		return nil, fmt.Errorf("cert: expected top-level structure")
	}
	if err := r.Enter(); err != nil {
		return nil, err
	}
	c := &Cert{}
	for r.Next() {
		tag := r.Tag()
		if tag.Kind != tlv.KindContext {
			return nil, fmt.Errorf("cert: non-context tag in certificate")
		}
		var err error
		switch tag.Num {
		case tagSerialNumber:
			c.SerialNumber, err = r.Bytes()
		case tagSigAlgo:
			c.SigAlgo, err = r.Uint()
		case tagIssuer:
			c.Issuer, err = decodeDN(r)
		case tagNotBefore:
			c.NotBefore, err = readU32(r)
		case tagNotAfter:
			c.NotAfter, err = readU32(r)
		case tagSubject:
			c.Subject, err = decodeDN(r)
		case tagPubKeyAlgo:
			c.PubKeyAlgo, err = r.Uint()
		case tagCurveID:
			c.CurveID, err = r.Uint()
		case tagPubKey:
			c.PublicKey, err = r.Bytes()
		case tagExtensions:
			c.Extensions, err = decodeExtensions(r)
		case tagSignature:
			c.Signature, err = r.Bytes()
		default:
			return nil, fmt.Errorf("cert: unknown top-level tag %d", tag.Num)
		}
		if err != nil {
			return nil, fmt.Errorf("cert: tag %d: %w", tag.Num, err)
		}
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return c, nil
}

func readU32(r *tlv.Reader) (uint32, error) {
	v, err := r.Uint()
	if err != nil {
		return 0, err
	}
	if v > 0xFFFFFFFF {
		return 0, fmt.Errorf("cert: value %d exceeds uint32", v)
	}
	return uint32(v), nil
}

func decodeDN(r *tlv.Reader) (DN, error) {
	if r.Type() != tlv.TypeList {
		return DN{}, fmt.Errorf("cert: DN is not a list")
	}
	if err := r.Enter(); err != nil {
		return DN{}, err
	}
	var dn DN
	for r.Next() {
		tag := r.Tag()
		if tag.Kind != tlv.KindContext || tag.Num > 0xFF {
			return DN{}, fmt.Errorf("cert: bad DN attribute tag")
		}
		a := Attr{Tag: uint8(tag.Num) &^ printableFlag}
		switch r.Type() {
		case tlv.TypeString:
			a.IsString = true
			a.Printable = uint8(tag.Num)&printableFlag != 0
			s, err := r.String()
			if err != nil {
				return DN{}, err
			}
			a.String = s
		case tlv.TypeUint:
			v, err := r.Uint()
			if err != nil {
				return DN{}, err
			}
			a.Value = v
		default:
			return DN{}, fmt.Errorf("cert: unexpected DN attribute type %v", r.Type())
		}
		dn.Attrs = append(dn.Attrs, a)
	}
	return dn, nil
}

func decodeExtensions(r *tlv.Reader) (Extensions, error) {
	if r.Type() != tlv.TypeList {
		return Extensions{}, fmt.Errorf("cert: extensions is not a list")
	}
	if err := r.Enter(); err != nil {
		return Extensions{}, err
	}
	var ext Extensions
	for r.Next() {
		tag := r.Tag()
		var err error
		switch tag.Num {
		case extBasicConstraints:
			ext.BasicConstraints, err = decodeBasicConstraints(r)
		case extKeyUsage:
			var v uint64
			if v, err = r.Uint(); err == nil {
				u := uint16(v)
				ext.KeyUsage = &u
			}
		case extExtendedKeyUsage:
			ext.ExtKeyUsage, err = decodeUintArray(r)
		case extSubjectKeyID:
			ext.SubjectKeyID, err = r.Bytes()
		case extAuthorityKeyID:
			ext.AuthorityKeyID, err = r.Bytes()
		case extFutureExtension:
			_, err = r.Bytes() // preserved opaquely; not re-modeled
		default:
			return Extensions{}, fmt.Errorf("cert: unknown extension tag %d", tag.Num)
		}
		if err != nil {
			return Extensions{}, err
		}
	}
	return ext, nil
}

func decodeBasicConstraints(r *tlv.Reader) (*BasicConstraints, error) {
	if r.Type() != tlv.TypeStructure {
		return nil, fmt.Errorf("cert: basic-constraints is not a structure")
	}
	if err := r.Enter(); err != nil {
		return nil, err
	}
	bc := &BasicConstraints{}
	for r.Next() {
		switch r.Tag().Num {
		case bcIsCA:
			v, err := r.Bool()
			if err != nil {
				return nil, err
			}
			bc.IsCA = v
		case bcPathLen:
			v, err := r.Uint()
			if err != nil {
				return nil, err
			}
			p := uint8(v)
			bc.PathLen = &p
		}
	}
	return bc, nil
}

func decodeUintArray(r *tlv.Reader) ([]uint8, error) {
	if r.Type() != tlv.TypeArray {
		return nil, fmt.Errorf("cert: expected array")
	}
	if err := r.Enter(); err != nil {
		return nil, err
	}
	var out []uint8
	for r.Next() {
		v, err := r.Uint()
		if err != nil {
			return nil, err
		}
		out = append(out, uint8(v))
	}
	return out, nil
}
