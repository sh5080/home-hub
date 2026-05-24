package matter

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/sh5080/home-hub/internal/domain"
)

// Poller periodically reads the position of native Matter devices and publishes
// state-changed events, so HomeKit and automations reflect real device state
// without a command having caused the change (e.g. a blind moved by its own
// schedule). Delegated devices expose no read-back and are skipped.
type Poller struct {
	reg      *Registry
	publish  func(domain.Event)
	interval time.Duration
	log      *slog.Logger
	last     map[string]int
}

// NewPoller builds a poller over reg. publish receives each state change
// (typically bus.PublishEvent).
func NewPoller(reg *Registry, publish func(domain.Event), interval time.Duration, log *slog.Logger) *Poller {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Poller{reg: reg, publish: publish, interval: interval, log: log, last: make(map[string]int)}
}

// Start polls until ctx is cancelled.
func (p *Poller) Start(ctx context.Context) error {
	t := time.NewTicker(p.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			p.pollOnce()
		}
	}
}

// pollOnce reads every readable device once, publishing only genuine changes.
func (p *Poller) pollOnce() {
	for _, id := range p.reg.IDs() {
		d, ok := p.reg.Get(id)
		if !ok {
			continue
		}
		pct, err := d.LiftPercent()
		if errors.Is(err, ErrUnsupported) {
			continue // delegated device: no read-back
		}
		if err != nil {
			if p.log != nil {
				p.log.Error("matter poll", "device", id, "err", err)
			}
			continue
		}
		if prev, seen := p.last[id]; seen && prev == pct {
			continue // unchanged since last poll
		}
		p.last[id] = pct
		// NOTE: pct is the driver's lift percent, mapped directly onto the
		// HomeKit-oriented Position. The orientation caveat in GoMatterDriver
		// applies here too; keep both directions consistent when validated.
		p.publish(domain.Event{
			DeviceID: id,
			Kind:     domain.EventStateChanged,
			State:    domain.State{Position: domain.IntPtr(pct)},
		})
	}
}
