package matter

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/sh5080/go-matter/cluster"
)

// ReconnectingDriver is a Driver over a native go-matter device that dials
// lazily and re-dials when the session dies (device reboot, network blip,
// device unreachable at hub startup). This replaces "fall back to delegated
// forever" behavior: a temporarily offline device recovers by itself.
type ReconnectingDriver struct {
	cfg GoMatterConfig
	log *slog.Logger

	dialTimeout time.Duration

	mu  sync.Mutex
	drv *GoMatterDriver // nil when disconnected
}

var _ Driver = (*ReconnectingDriver)(nil)

// NewReconnecting builds a reconnecting driver. It does not dial yet; the
// first operation (or poll) establishes the session.
func NewReconnecting(cfg GoMatterConfig, log *slog.Logger) *ReconnectingDriver {
	return &ReconnectingDriver{cfg: cfg, log: log, dialTimeout: 15 * time.Second}
}

// driver returns the live driver, dialing if needed. Caller holds r.mu.
func (r *ReconnectingDriver) driver() (*GoMatterDriver, error) {
	if r.drv != nil {
		return r.drv, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.dialTimeout)
	defer cancel()
	drv, err := DialGoMatter(ctx, r.cfg)
	if err != nil {
		return nil, err
	}
	if r.log != nil {
		r.log.Info("matter session established", "node", r.cfg.NodeID)
	}
	r.drv = drv
	return drv, nil
}

// drop discards a (presumed dead) session so the next operation re-dials.
// Caller holds r.mu.
func (r *ReconnectingDriver) drop() {
	if r.drv != nil {
		_ = r.drv.Shutdown()
		r.drv = nil
	}
}

// do runs op on a live session; on failure it re-dials once and retries, so a
// stale session (e.g. the device rebooted since the last command) costs one
// extra round trip instead of an error to the user.
func (r *ReconnectingDriver) do(op func(*GoMatterDriver) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	drv, err := r.driver()
	if err != nil {
		return err
	}
	firstErr := op(drv)
	if firstErr == nil {
		return nil
	}
	if errors.Is(firstErr, cluster.ErrNull) {
		return firstErr // healthy session, value just unknown — not a failure
	}
	if r.log != nil {
		r.log.Warn("matter op failed; re-dialing", "node", r.cfg.NodeID, "err", firstErr)
	}
	r.drop()
	drv, err = r.driver()
	if err != nil {
		return firstErr // report the original failure; redial failed too
	}
	if err := op(drv); err != nil {
		r.drop()
		return err
	}
	return nil
}

// Open moves the covering toward open.
func (r *ReconnectingDriver) Open() error {
	return r.do(func(d *GoMatterDriver) error { return d.Open() })
}

// Close moves the covering toward closed.
func (r *ReconnectingDriver) Close() error {
	return r.do(func(d *GoMatterDriver) error { return d.Close() })
}

// SetLiftPercent moves the covering to p (domain orientation).
func (r *ReconnectingDriver) SetLiftPercent(p int) error {
	return r.do(func(d *GoMatterDriver) error { return d.SetLiftPercent(p) })
}

// LiftPercent reads the current position (domain orientation).
func (r *ReconnectingDriver) LiftPercent() (int, error) {
	var pct int
	err := r.do(func(d *GoMatterDriver) error {
		var e error
		pct, e = d.LiftPercent()
		return e
	})
	return pct, err
}

// Shutdown releases any live session.
func (r *ReconnectingDriver) Shutdown() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.drop()
	return nil
}
