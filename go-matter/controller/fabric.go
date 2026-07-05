package controller

import (
	"crypto/rand"
	"crypto/sha1"
	"fmt"
	"time"

	"github.com/sh5080/go-matter/cert"
	"github.com/sh5080/go-matter/crypto"
)

// matterEpoch is 2000-01-01T00:00:00Z as a Unix timestamp: Matter certificate
// validity counts seconds from it (Spec 6.5.5.1).
const matterEpoch = 946684800

// keyID is the X.509 method-1 key identifier: SHA-1 of the public key.
func keyID(pub []byte) []byte {
	sum := sha1.Sum(pub)
	return sum[:]
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

// GenerateFabric creates a brand-new fabric: a root CA (RCAC + signing key),
// the controller's operational identity (NOC + key), and a random IPK. The
// result contains the CA private key, so the hub can later issue NOCs to
// devices it commissions — Save it with care.
func GenerateFabric(fabricID, controllerNodeID uint64) (StoredFabric, error) {
	if fabricID == 0 || controllerNodeID == 0 {
		return StoredFabric{}, fmt.Errorf("controller: fabric and controller node ids must be non-zero")
	}
	rootKey, err := crypto.GenerateP256Scalar()
	if err != nil {
		return StoredFabric{}, err
	}
	rootPub, err := crypto.PublicFromScalar(rootKey)
	if err != nil {
		return StoredFabric{}, err
	}
	rootIDBytes, err := randomBytes(8)
	if err != nil {
		return StoredFabric{}, err
	}
	rootID := uint64(0)
	for _, b := range rootIDBytes {
		rootID = rootID<<8 | uint64(b)
	}

	notBefore := uint32(time.Now().Unix() - matterEpoch - 24*3600)
	serial, err := randomBytes(8)
	if err != nil {
		return StoredFabric{}, err
	}

	rootDN := cert.DN{Attrs: []cert.Attr{{Tag: cert.DNMatterRCACID, Value: rootID}}}
	pathLen := uint8(1)
	keyUsageCA := uint16(0x60) // keyCertSign | cRLSign
	rcac := &cert.Cert{
		SerialNumber: serial, SigAlgo: 1,
		Issuer: rootDN, Subject: rootDN,
		NotBefore: notBefore, NotAfter: 0, // 0 = no well-defined expiry
		PubKeyAlgo: 1, CurveID: 1, PublicKey: rootPub,
		Extensions: cert.Extensions{
			BasicConstraints: &cert.BasicConstraints{IsCA: true, PathLen: &pathLen},
			KeyUsage:         &keyUsageCA,
			SubjectKeyID:     keyID(rootPub),
			AuthorityKeyID:   keyID(rootPub),
		},
	}
	rcacTLV, err := rcac.SignAndEncode(rootKey)
	if err != nil {
		return StoredFabric{}, fmt.Errorf("controller: sign RCAC: %w", err)
	}

	ctrlKey, err := crypto.GenerateP256Scalar()
	if err != nil {
		return StoredFabric{}, err
	}
	ctrlPub, err := crypto.PublicFromScalar(ctrlKey)
	if err != nil {
		return StoredFabric{}, err
	}
	s := StoredFabric{
		FabricID:      fabricID,
		RootPublicKey: rootPub,
		RCAC:          rcacTLV,
		RootKey:       rootKey,
		ControllerKey: ctrlKey,
	}
	if s.IPK, err = randomBytes(16); err != nil {
		return StoredFabric{}, err
	}
	if s.ControllerNOC, err = s.IssueNOC(ctrlPub, controllerNodeID); err != nil {
		return StoredFabric{}, err
	}
	return s, nil
}

// IssueNOC signs a node operational certificate for pub with the fabric's CA
// key, assigning nodeID on this fabric. Requires RootKey (a fabric imported
// from chip-tool does not carry the CA key and cannot issue).
func (s StoredFabric) IssueNOC(pub []byte, nodeID uint64) ([]byte, error) {
	if len(s.RootKey) == 0 {
		return nil, fmt.Errorf("controller: fabric store has no CA key; cannot issue NOCs")
	}
	root, err := cert.Decode(s.RCAC)
	if err != nil {
		return nil, fmt.Errorf("controller: decode RCAC: %w", err)
	}
	serial, err := randomBytes(8)
	if err != nil {
		return nil, err
	}
	keyUsageNOC := uint16(0x01) // digitalSignature
	noc := &cert.Cert{
		SerialNumber: serial, SigAlgo: 1,
		Issuer:    root.Subject,
		NotBefore: root.NotBefore, NotAfter: root.NotAfter,
		Subject: cert.DN{Attrs: []cert.Attr{
			{Tag: cert.DNMatterNodeID, Value: nodeID},
			{Tag: cert.DNMatterFabricID, Value: s.FabricID},
		}},
		PubKeyAlgo: 1, CurveID: 1, PublicKey: pub,
		Extensions: cert.Extensions{
			BasicConstraints: &cert.BasicConstraints{IsCA: false},
			KeyUsage:         &keyUsageNOC,
			ExtKeyUsage:      []uint8{2, 1}, // clientAuth, serverAuth
			SubjectKeyID:     keyID(pub),
			AuthorityKeyID:   root.Extensions.SubjectKeyID,
		},
	}
	return noc.SignAndEncode(s.RootKey)
}
