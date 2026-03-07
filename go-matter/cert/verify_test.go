package cert

import (
	"bytes"
	"testing"
)

// Real CHIP NOC "Node01_01" (issued under an ICA), in TLV and X.509 DER form.
// Exercises node-id/fabric-id DN attributes, extended-key-usage, and a non-CA
// basic-constraints (empty SEQUENCE) — paths the RCAC vector does not cover.
const (
	node0101ChipHex = "1530010818e969ba0e089e232402013703271303000000cacacaca182604ef171b2726056eb5b94c3706271101000100dededede27151d0000000000b0fa1824070124080130094104bcf6580d2d71e14416651f7c311b5efcf9aec0a8c10af80927844c240f51a8eb23fa0744138887ac1e73cb72a054b6a0db0622aa807071016313b1596c8552cf370a3501280118240201360304020401183004146967c912f8a3e689556f899b65d76f53fa65c7b6300514440cc69231c4cb5b37942426f81bbe24b7ef345c18300b40ce6ef393cbbc94f80ee290cb3c3d373335bab95907734d99d384a62a373b8484e1d41a04c3140faa19e8a2b99b0c61e33c27ea913973e45b5bc6e39c270dac5318"

	node0101DERHex = "308201e130820186a003020102020818e969ba0e089e23300a06082a8648ce3d04030230223120301e060a2b0601040182a27c01030c1043414341434143413030303030303033301e170d3230313031353134323334335a170d3430313031353134323334325a30443120301e060a2b0601040182a27c01010c10444544454445444530303031303030313120301e060a2b0601040182a27c01050c10464142303030303030303030303031443059301306072a8648ce3d020106082a8648ce3d03010703420004bcf6580d2d71e14416651f7c311b5efcf9aec0a8c10af80927844c240f51a8eb23fa0744138887ac1e73cb72a054b6a0db0622aa807071016313b1596c8552cfa38183308180300c0603551d130101ff04023000300e0603551d0f0101ff04040302078030200603551d250101ff0416301406082b0601050507030206082b06010505070301301d0603551d0e041604146967c912f8a3e689556f899b65d76f53fa65c7b6301f0603551d23041830168014440cc69231c4cb5b37942426f81bbe24b7ef345c300a06082a8648ce3d0403020349003046022100ce6ef393cbbc94f80ee290cb3c3d373335bab95907734d99d384a62a373b8484022100e1d41a04c3140faa19e8a2b99b0c61e33c27ea913973e45b5bc6e39c270dac53"
)

// derContent returns the content octets of the first DER element in b.
func derContent(b []byte) []byte {
	i := 1
	l := int(b[i])
	i++
	if l&0x80 != 0 {
		n := l & 0x7f
		l = 0
		for k := 0; k < n; k++ {
			l = l<<8 | int(b[i])
			i++
		}
	}
	return b[i : i+l]
}

// firstElement returns the first complete DER element (tag||len||content) in b.
func firstElement(b []byte) []byte {
	i := 1
	l := int(b[i])
	i++
	if l&0x80 != 0 {
		n := l & 0x7f
		l = 0
		for k := 0; k < n; k++ {
			l = l<<8 | int(b[i])
			i++
		}
	}
	return b[:i+l]
}

// tbsOf extracts the TBSCertificate (first element inside the cert SEQUENCE).
func tbsOf(certDER []byte) []byte { return firstElement(derContent(certDER)) }

func TestTBSMatchesReference(t *testing.T) {
	cases := []struct{ name, chip, der string }{
		{"root01", root01ChipHex, root01DERHex},
		{"node01_01", node0101ChipHex, node0101DERHex},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Decode(decodeHex(t, tc.chip))
			if err != nil {
				t.Fatal(err)
			}
			tbs, err := c.tbsDER()
			if err != nil {
				t.Fatal(err)
			}
			want := tbsOf(decodeHex(t, tc.der))
			if !bytes.Equal(tbs, want) {
				t.Fatalf("TBS mismatch\n got  %x\n want %x", tbs, want)
			}
		})
	}
}

func TestVerifyRoot01SelfSignature(t *testing.T) {
	c, err := Decode(decodeHex(t, root01ChipHex))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.VerifySignature(c.PublicKey); err != nil {
		t.Fatalf("self-signature should verify: %v", err)
	}

	// Tampering the TBS (serial) must make verification fail.
	tampered := *c
	tampered.SerialNumber = append([]byte{}, c.SerialNumber...)
	tampered.SerialNumber[0] ^= 1
	if err := tampered.VerifySignature(c.PublicKey); err == nil {
		t.Fatal("verification passed on a tampered certificate")
	}
	// A wrong issuer key must fail too.
	wrong := append([]byte{}, c.PublicKey...)
	wrong[1] ^= 1
	if err := c.VerifySignature(wrong); err == nil {
		t.Fatal("verification passed under the wrong issuer key")
	}
}

func TestNode0101CodecRoundTrip(t *testing.T) {
	orig := decodeHex(t, node0101ChipHex)
	c, err := Decode(orig)
	if err != nil {
		t.Fatal(err)
	}
	if id, ok := c.Subject.NodeID(); !ok || id != 0xdededede00010001 {
		t.Fatalf("node id = %#x (ok=%v)", id, ok)
	}
	if fid, ok := c.Subject.FabricID(); !ok || fid != 0xfab000000000001d {
		t.Fatalf("fabric id = %#x (ok=%v)", fid, ok)
	}
	if len(c.Extensions.ExtKeyUsage) != 2 {
		t.Fatalf("expected 2 EKU purposes, got %v", c.Extensions.ExtKeyUsage)
	}
	reenc, err := c.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(orig, reenc) {
		t.Fatalf("NOC re-encode not byte-exact")
	}
}
