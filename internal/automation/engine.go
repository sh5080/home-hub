// Package automation evaluates rules over bus events and issues commands.
package automation

import (
	"context"
	"log/slog"

	"github.com/sh5080/home-hub/internal/bus"
	"github.com/sh5080/home-hub/internal/domain"
)

// Rule reacts to an event and optionally returns commands to run.
type Rule func(e domain.Event) []domain.Command

// Engine runs rules against the event stream.
type Engine struct {
	bus   *bus.Bus
	log   *slog.Logger
	rules []Rule
}

// New builds an automation engine.
func New(b *bus.Bus, log *slog.Logger) *Engine {
	return &Engine{bus: b, log: log}
}

// Add registers a rule.
func (en *Engine) Add(r Rule) { en.rules = append(en.rules, r) }

// Start consumes events and applies matching rules until ctx is cancelled.
func (en *Engine) Start(ctx context.Context) error {
	events := en.bus.SubscribeEvents()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case e := <-events:
			for _, r := range en.rules {
				for _, cmd := range r(e) {
					en.log.Info("automation -> command", "device", cmd.DeviceID, "action", cmd.Action)
					en.bus.PublishCommand(cmd)
				}
			}
		}
	}
}
