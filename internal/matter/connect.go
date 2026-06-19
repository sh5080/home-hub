package matter

import (
	"context"
	"fmt"

	"github.com/sh5080/go-matter/cluster"
	"github.com/sh5080/go-matter/controller"
	"github.com/sh5080/go-matter/im"
)

// GoMatterConfig describes how to reach a natively-controlled Matter device.
type GoMatterConfig struct {
	FabricStore string // path to the StoredFabric JSON (fabric + controller identity)
	NodeID      uint64 // device operational node id
	Address     string // host:port, e.g. "192.168.1.20:5540"; empty means resolve over mDNS
	Endpoint    uint16 // window-covering endpoint
}

// dial loads the fabric credentials and establishes a CASE session to the
// device. When Address is set it dials directly; otherwise the device is
// resolved by node id over mDNS.
func dial(ctx context.Context, cfg GoMatterConfig) (*controller.Session, error) {
	stored, err := controller.LoadFabric(cfg.FabricStore)
	if err != nil {
		return nil, fmt.Errorf("matter: load fabric %q: %w", cfg.FabricStore, err)
	}
	fabric, err := stored.Fabric()
	if err != nil {
		return nil, err
	}
	ctrl := controller.New(fabric, stored.Identity())
	if cfg.Address != "" {
		return ctrl.DialAddr(ctx, cfg.NodeID, cfg.Address)
	}
	return ctrl.Dial(ctx, cfg.NodeID)
}

// DialGoMatter connects to the device and returns a driver bound to that live
// session. The returned driver owns the session; call Shutdown to release it.
func DialGoMatter(ctx context.Context, cfg GoMatterConfig) (*GoMatterDriver, error) {
	sess, err := dial(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("matter: dial node 0x%x: %w", cfg.NodeID, err)
	}
	return NewGoMatterDriver(sess, cfg.Endpoint), nil
}

// SubscribeGoMatter opens a dedicated session and subscribes to the device's
// lift position for push state updates. Commands and subscriptions must not
// share a session (a subscription owns the receive path), so this is a second
// session distinct from DialGoMatter's. The returned session owns the
// subscription; Close it to stop watching.
func SubscribeGoMatter(ctx context.Context, cfg GoMatterConfig) (*controller.Session, *controller.Subscription, error) {
	sess, err := dial(ctx, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("matter: subscribe-dial node 0x%x: %w", cfg.NodeID, err)
	}
	// Report at least once a minute, at most once a second.
	sub, err := sess.Subscribe(ctx, []im.AttributePath{cluster.LiftPositionAttribute(cfg.Endpoint)}, 1, 60)
	if err != nil {
		sess.Close()
		return nil, nil, fmt.Errorf("matter: subscribe node 0x%x: %w", cfg.NodeID, err)
	}
	return sess, sub, nil
}
