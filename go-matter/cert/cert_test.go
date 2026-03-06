package cert

import (
	"bytes"
	"testing"
)

func u16(v uint16) *uint16 { return &v }
func u8(v uint8) *uint8     { return &v }

// noc builds a leaf (NOC-shaped) certificate exercising every modeled field.
func noc() *Cert {
	return &Cert{
		SerialNumber: []byte{0x01, 0x02, 0x03, 0x04},
		SigAlgo:      1,
		Issuer:       DN{Attrs: []Attr{{Tag: DNMatterRCACID, Value: 0xCA00000000000001}}},
		NotBefore:    0x2A2A2A2A,
		NotAfter:     0x3B3B3B3B,
		Subject: DN{Attrs: []Attr{
			{Tag: DNMatterNodeID, Value: 0x0000000000001234},
			{Tag: DNMatterFabricID, Value: 0x000000000000ABCD},
			{Tag: DNCommonName, IsString: true, Printable: true, String: "node-1234"},
		}},
		PubKeyAlgo: 1,
		CurveID:    1,
		PublicKey:  append([]byte{0x04}, bytes.Repeat([]byte{0xAB}, 64)...),
		Extensions: Extensions{
			BasicConstraints: &BasicConstraints{IsCA: false},
			KeyUsage:         u16(0x01),
			ExtKeyUsage:      []uint8{2, 1},
			SubjectKeyID:     bytes.Repeat([]byte{0x11}, 20),
			AuthorityKeyID:   bytes.Repeat([]byte{0x22}, 20),
		},
		Signature: bytes.Repeat([]byte{0x33}, 64),
	}
}

// rcac builds a self-signed root (CA with a path-length constraint).
func rcac() *Cert {
	return &Cert{
		SerialNumber: []byte{0x99},
		SigAlgo:      1,
		Issuer:       DN{Attrs: []Attr{{Tag: DNMatterRCACID, Value: 0xCA00000000000001}}},
		NotBefore:    1,
		NotAfter:     2,
		Subject:      DN{Attrs: []Attr{{Tag: DNMatterRCACID, Value: 0xCA00000000000001}}},
		PubKeyAlgo:   1,
		CurveID:      1,
		PublicKey:    append([]byte{0x04}, bytes.Repeat([]byte{0xCD}, 64)...),
		Extensions: Extensions{
			BasicConstraints: &BasicConstraints{IsCA: true, PathLen: u8(1)},
			KeyUsage:         u16(0x60), // keyCertSign | cRLSign
			SubjectKeyID:     bytes.Repeat([]byte{0xAA}, 20),
			AuthorityKeyID:   bytes.Repeat([]byte{0xAA}, 20),
		},
		Signature: bytes.Repeat([]byte{0x44}, 64),
	}
}

func TestCertRoundTrip(t *testing.T) {
	for name, c := range map[string]*Cert{"noc": noc(), "rcac": rcac()} {
		t.Run(name, func(t *testing.T) {
			enc, err := c.Encode()
			if err != nil {
				t.Fatal(err)
			}
			got, err := Decode(enc)
			if err != nil {
				t.Fatal(err)
			}
			reenc, err := got.Encode()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(enc, reenc) {
				t.Fatalf("round trip not byte-exact:\n a=%x\n b=%x", enc, reenc)
			}
		})
	}
}

func TestCertFieldExtraction(t *testing.T) {
	got, err := Decode(mustEncode(t, noc()))
	if err != nil {
		t.Fatal(err)
	}
	if id, ok := got.Subject.NodeID(); !ok || id != 0x1234 {
		t.Fatalf("node id = %#x (ok=%v)", id, ok)
	}
	if fid, ok := got.Subject.FabricID(); !ok || fid != 0xABCD {
		t.Fatalf("fabric id = %#x (ok=%v)", fid, ok)
	}
	if rid, ok := got.Issuer.uintAttr(DNMatterRCACID); !ok || rid != 0xCA00000000000001 {
		t.Fatalf("issuer rcac id = %#x (ok=%v)", rid, ok)
	}
	if bc := got.Extensions.BasicConstraints; bc == nil || bc.IsCA {
		t.Fatal("basic constraints not decoded as non-CA")
	}
	if ku := got.Extensions.KeyUsage; ku == nil || *ku != 1 {
		t.Fatalf("key usage = %v", got.Extensions.KeyUsage)
	}
	if len(got.PublicKey) != 65 || got.PublicKey[0] != 0x04 {
		t.Fatalf("public key malformed: %x", got.PublicKey)
	}
}

func TestCARoundTripPreservesPathLen(t *testing.T) {
	got, err := Decode(mustEncode(t, rcac()))
	if err != nil {
		t.Fatal(err)
	}
	bc := got.Extensions.BasicConstraints
	if bc == nil || !bc.IsCA || bc.PathLen == nil || *bc.PathLen != 1 {
		t.Fatalf("basic constraints = %+v", bc)
	}
}

func mustEncode(t *testing.T, c *Cert) []byte {
	t.Helper()
	b, err := c.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return b
}
