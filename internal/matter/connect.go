package matter

import (
	"context"
	"fmt"

	"github.com/sh5080/go-matter/controller"
)

// GoMatterConfig describes how to reach a natively-controlled Matter device.
type GoMatterConfig struct {
	FabricStore string // path to the StoredFabric JSON (fabric + controller identity)
	NodeID      uint64 // device operational node id
	Address     string // host:port, e.g. "192.168.1.20:5540"
	Endpoint    uint16 // window-covering endpoint
}

// DialGoMatter loads the fabric credentials, establishes a CASE session to the
// device, and returns a driver bound to that live session. The returned driver
// owns the session; call Shutdown to release it.
func DialGoMatter(ctx context.Context, cfg GoMatterConfig) (*GoMatterDriver, error) {
	stored, err := controller.LoadFabric(cfg.FabricStore)
	if err != nil {
		return nil, fmt.Errorf("matter: load fabric %q: %w", cfg.FabricStore, err)
	}
	fabric, err := stored.Fabric()
	if err != nil {
		return nil, err
	}
	sess, err := controller.New(fabric, stored.Identity()).DialAddr(ctx, cfg.NodeID, cfg.Address)
	if err != nil {
		return nil, fmt.Errorf("matter: dial %s: %w", cfg.Address, err)
	}
	return NewGoMatterDriver(sess, cfg.Endpoint), nil
}
