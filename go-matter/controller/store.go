package controller

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/sh5080/go-matter/casesession"
	"github.com/sh5080/go-matter/cert"
)

// StoredFabric is the on-disk representation of a fabric and the controller's
// operational identity within it. It contains a private key, so it must be
// written with owner-only permissions.
type StoredFabric struct {
	FabricID      uint64 `json:"fabricId"`
	IPK           []byte `json:"ipk"`           // 16-byte identity protection key
	RootPublicKey []byte `json:"rootPublicKey"` // 65-byte uncompressed point
	RCAC          []byte `json:"rcac"`          // root certificate (Matter TLV)
	ControllerNOC []byte `json:"controllerNoc"` // controller NOC (Matter TLV)
	ControllerKey []byte `json:"controllerKey"` // 32-byte P-256 operational scalar (secret)
}

// Save writes the fabric to path with 0600 permissions.
func (s StoredFabric) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// LoadFabric reads a StoredFabric from path.
func LoadFabric(path string) (StoredFabric, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return StoredFabric{}, err
	}
	var s StoredFabric
	if err := json.Unmarshal(data, &s); err != nil {
		return StoredFabric{}, fmt.Errorf("controller: parse fabric store: %w", err)
	}
	return s, nil
}

// Fabric builds the runtime Fabric, decoding the stored RCAC.
func (s StoredFabric) Fabric() (casesession.Fabric, error) {
	rcac, err := cert.Decode(s.RCAC)
	if err != nil {
		return casesession.Fabric{}, fmt.Errorf("controller: decode RCAC: %w", err)
	}
	return casesession.Fabric{IPK: s.IPK, FabricID: s.FabricID, RootPubKey: s.RootPublicKey, RCAC: rcac}, nil
}

// Identity builds the controller's operational identity.
func (s StoredFabric) Identity() casesession.Identity {
	return casesession.Identity{NOC: s.ControllerNOC, OpKey: s.ControllerKey}
}
