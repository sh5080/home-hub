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
	Address     string // host:port, e.g. "192.168.1.20:5540"; empty means resolve over mDNS
	Endpoint    uint16 // window-covering endpoint
}

// DialGoMatter loads the fabric credentials, establishes a CASE session to the
// device, and returns a driver bound to that live session. When Address is set
// it dials directly; otherwise the device is resolved by node id over mDNS. The
// returned driver owns the session; call Shutdown to release it.
func DialGoMatter(ctx context.Context, cfg GoMatterConfig) (*GoMatterDriver, error) {
	stored, err := controller.LoadFabric(cfg.FabricStore)
	if err != nil {
		return nil, fmt.Errorf("matter: load fabric %q: %w", cfg.FabricStore, err)
	}
	fabric, err := stored.Fabric()
	if err != nil {
		return nil, err
	}
	ctrl := controller.New(fabric, stored.Identity())

	var sess *controller.Session
	if cfg.Address != "" {
		sess, err = ctrl.DialAddr(ctx, cfg.NodeID, cfg.Address)
	} else {
		sess, err = ctrl.Dial(ctx, cfg.NodeID)
	}
	if err != nil {
		return nil, fmt.Errorf("matter: dial node 0x%x: %w", cfg.NodeID, err)
	}
	return NewGoMatterDriver(sess, cfg.Endpoint), nil
}
