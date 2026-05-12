package controller

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/sh5080/go-matter/cert"
)

func TestStoredFabricRoundTrip(t *testing.T) {
	s := StoredFabric{
		FabricID:      fabricID,
		IPK:           bytes.Repeat([]byte{0x77}, 16),
		RootPublicKey: bytes.Repeat([]byte{0x04}, 65),
		RCAC:          []byte{0x15, 0x18},
		ControllerNOC: []byte{0x15, 0x18},
		ControllerKey: bytes.Repeat([]byte{0x11}, 32),
	}
	path := filepath.Join(t.TempDir(), "fabric.json")
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}

	// Private key must be stored owner-only.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("permissions = %o, want 600", perm)
	}

	got, err := LoadFabric(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.FabricID != s.FabricID || !bytes.Equal(got.IPK, s.IPK) ||
		!bytes.Equal(got.ControllerKey, s.ControllerKey) || !bytes.Equal(got.RootPublicKey, s.RootPublicKey) {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

func TestStoredFabricBuild(t *testing.T) {
	rootKey, rootPub := genKey(t)
	rcac := &cert.Cert{
		SerialNumber: []byte{0x01}, SigAlgo: 1,
		Issuer:    cert.DN{Attrs: []cert.Attr{{Tag: cert.DNMatterRCACID, Value: rootID}}},
		NotBefore: 0x271b17ef, NotAfter: 0x4cb9b56e,
		Subject:   cert.DN{Attrs: []cert.Attr{{Tag: cert.DNMatterRCACID, Value: rootID}}},
		PubKeyAlgo: 1, CurveID: 1, PublicKey: rootPub,
		Extensions: cert.Extensions{
			BasicConstraints: &cert.BasicConstraints{IsCA: true, PathLen: u8p(1)},
			KeyUsage:         u16p(0x60),
			SubjectKeyID:     bytes.Repeat([]byte{0xAA}, 20),
			AuthorityKeyID:   bytes.Repeat([]byte{0xAA}, 20),
		},
	}
	rcacTLV, err := rcac.SignAndEncode(rootKey)
	if err != nil {
		t.Fatal(err)
	}
	ctrlKey, ctrlPub := genKey(t)
	ctrlNOC, err := noc(ctrlPub, ctrlNode).SignAndEncode(rootKey)
	if err != nil {
		t.Fatal(err)
	}

	s := StoredFabric{
		FabricID: fabricID, IPK: bytes.Repeat([]byte{0x77}, 16), RootPublicKey: rootPub,
		RCAC: rcacTLV, ControllerNOC: ctrlNOC, ControllerKey: ctrlKey,
	}
	fab, err := s.Fabric()
	if err != nil {
		t.Fatal(err)
	}
	if fab.FabricID != fabricID || !bytes.Equal(fab.RootPubKey, rootPub) || fab.RCAC == nil {
		t.Fatalf("fabric = %+v", fab)
	}
	id := s.Identity()
	if !bytes.Equal(id.NOC, ctrlNOC) || !bytes.Equal(id.OpKey, ctrlKey) {
		t.Fatalf("identity mismatch")
	}
}
