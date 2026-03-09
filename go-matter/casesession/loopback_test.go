package casesession

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"

	"github.com/sh5080/go-matter/cert"
	"github.com/sh5080/go-matter/crypto"
)

const (
	testFabricID = 0xFAB000000000001D
	testRootID   = 0xCACACACA00000001
	initNodeID   = 0x1111000000000001
	respNodeID   = 0x2222000000000002
)

func u16p(v uint16) *uint16 { return &v }
func u8p(v uint8) *uint8     { return &v }

func genKey(t *testing.T) (scalar, pub []byte) {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	scalar = make([]byte, 32)
	k.D.FillBytes(scalar)
	if pub, err = crypto.PublicFromScalar(scalar); err != nil {
		t.Fatal(err)
	}
	return
}

func rcacTemplate(pub []byte) *cert.Cert {
	dn := cert.DN{Attrs: []cert.Attr{{Tag: cert.DNMatterRCACID, Value: testRootID}}}
	return &cert.Cert{
		SerialNumber: []byte{0x01}, SigAlgo: 1,
		Issuer: dn, NotBefore: 0x271b17ef, NotAfter: 0x4cb9b56e, Subject: dn,
		PubKeyAlgo: 1, CurveID: 1, PublicKey: pub,
		Extensions: cert.Extensions{
			BasicConstraints: &cert.BasicConstraints{IsCA: true, PathLen: u8p(1)},
			KeyUsage:         u16p(0x60),
			SubjectKeyID:     bytes.Repeat([]byte{0xAA}, 20),
			AuthorityKeyID:   bytes.Repeat([]byte{0xAA}, 20),
		},
	}
}

func nocTemplate(pub []byte, nodeID uint64) *cert.Cert {
	return &cert.Cert{
		SerialNumber: []byte{0x02}, SigAlgo: 1,
		Issuer:    cert.DN{Attrs: []cert.Attr{{Tag: cert.DNMatterRCACID, Value: testRootID}}},
		NotBefore: 0x271b17ef, NotAfter: 0x4cb9b56e,
		Subject: cert.DN{Attrs: []cert.Attr{
			{Tag: cert.DNMatterNodeID, Value: nodeID},
			{Tag: cert.DNMatterFabricID, Value: testFabricID},
		}},
		PubKeyAlgo: 1, CurveID: 1, PublicKey: pub,
		Extensions: cert.Extensions{
			BasicConstraints: &cert.BasicConstraints{IsCA: false},
			KeyUsage:         u16p(0x01),
			ExtKeyUsage:      []uint8{2, 1},
			SubjectKeyID:     bytes.Repeat([]byte{0xBB}, 20),
			AuthorityKeyID:   bytes.Repeat([]byte{0xAA}, 20),
		},
	}
}

// testFabric mints a self-signed root and two NOCs (initiator, responder) using
// the real signing and certificate paths.
func testFabric(t *testing.T) (Fabric, Identity, Identity) {
	rcacKey, rcacPub := genKey(t)
	rcacTLV, err := rcacTemplate(rcacPub).SignAndEncode(rcacKey)
	if err != nil {
		t.Fatal(err)
	}
	rcacDec, err := cert.Decode(rcacTLV)
	if err != nil {
		t.Fatal(err)
	}

	initKey, initPub := genKey(t)
	respKey, respPub := genKey(t)
	initNOC, err := nocTemplate(initPub, initNodeID).SignAndEncode(rcacKey)
	if err != nil {
		t.Fatal(err)
	}
	respNOC, err := nocTemplate(respPub, respNodeID).SignAndEncode(rcacKey)
	if err != nil {
		t.Fatal(err)
	}

	fabric := Fabric{
		IPK:        bytes.Repeat([]byte{0x77}, 16),
		FabricID:   testFabricID,
		RootPubKey: rcacPub,
		RCAC:       rcacDec,
	}
	return fabric, Identity{NOC: initNOC, OpKey: initKey}, Identity{NOC: respNOC, OpKey: respKey}
}

// TestCASELoopback runs a full Sigma1/2/3 handshake between an initiator and a
// responder built on real certificates, ECDH, and ECDSA signatures, and checks
// that both sides derive identical session keys. This validates the message
// flow, transcript hashing, key schedule, and signature/certificate handling
// end to end without hardware.
func TestCASELoopback(t *testing.T) {
	fabric, initID, respID := testFabric(t)

	initiator, err := NewInitiator(fabric, initID, respNodeID, 0x1001)
	if err != nil {
		t.Fatal(err)
	}
	responder, err := NewResponder(fabric, respID, 0x2002)
	if err != nil {
		t.Fatal(err)
	}

	s1, err := initiator.Sigma1()
	if err != nil {
		t.Fatalf("sigma1: %v", err)
	}
	s2, err := responder.HandleSigma1(s1)
	if err != nil {
		t.Fatalf("responder handle sigma1: %v", err)
	}
	s3, err := initiator.HandleSigma2(s2)
	if err != nil {
		t.Fatalf("initiator handle sigma2: %v", err)
	}
	if err := responder.HandleSigma3(s3); err != nil {
		t.Fatalf("responder handle sigma3: %v", err)
	}

	ik, err := initiator.SessionKeys()
	if err != nil {
		t.Fatal(err)
	}
	rk, err := responder.SessionKeys()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ik.I2R, rk.I2R) || !bytes.Equal(ik.R2I, rk.R2I) || !bytes.Equal(ik.AttestationChallenge, rk.AttestationChallenge) {
		t.Fatalf("session keys differ:\n init I2R=%x R2I=%x\n resp I2R=%x R2I=%x", ik.I2R, ik.R2I, rk.I2R, rk.R2I)
	}
	if bytes.Equal(ik.I2R, ik.R2I) {
		t.Fatal("I2R and R2I must differ")
	}
}

// TestCASERejectsWrongFabric ensures a responder on a different IPK cannot
// complete the handshake (the initiator's checks fail).
func TestCASERejectsTamperedSigma2(t *testing.T) {
	fabric, initID, respID := testFabric(t)
	initiator, _ := NewInitiator(fabric, initID, respNodeID, 1)
	responder, _ := NewResponder(fabric, respID, 2)

	s1, _ := initiator.Sigma1()
	s2, _ := responder.HandleSigma1(s1)
	s2[len(s2)-1] ^= 0xFF // corrupt the encrypted payload
	if _, err := initiator.HandleSigma2(s2); err == nil {
		t.Fatal("initiator accepted a tampered Sigma2")
	}
}
