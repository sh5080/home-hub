// Package homekit exposes hub devices to HomeKit controllers over HAP.
//
// TODO: back this with github.com/brutella/hap. For now it is a scaffold that
// wires into the event bus so the rest of the hub can be built against it.
package homekit

import (
	"context"
	"log/slog"

	"github.com/sh5080/home-hub/internal/bus"
	"github.com/sh5080/home-hub/internal/domain"
	"github.com/sh5080/home-hub/internal/registry"
)

// Bridge publishes accessories to HomeKit and forwards HomeKit-originated
// commands onto the bus. It also exposes virtual "trigger" switches used to
// delegate Matter devices to HomeKit automations (see internal/matter).
type Bridge struct {
	bus      *bus.Bus
	reg      *registry.Registry
	log      *slog.Logger
	triggers map[string]struct{}
}

// New builds a HomeKit bridge.
func New(b *bus.Bus, reg *registry.Registry, log *slog.Logger) *Bridge {
	return &Bridge{bus: b, reg: reg, log: log, triggers: make(map[string]struct{})}
}

// Name identifies the adapter.
func (br *Bridge) Name() string { return "homekit" }

// RegisterTrigger exposes a stateless virtual switch that HomeKit automations
// can react to, and returns a func that "presses" it.
func (br *Bridge) RegisterTrigger(id string) func() {
	br.triggers[id] = struct{}{}
	return func() {
		// TODO(hap): pulse the characteristic On->Off so a HomeKit automation fires.
		br.log.Info("virtual trigger pressed", "trigger", id)
	}
}

// Start publishes accessories and serves HAP until ctx is cancelled.
func (br *Bridge) Start(ctx context.Context) error {
	for _, d := range br.reg.List() {
		if d.Integration == domain.Matter {
			continue // Matter devices are delegated; only their triggers are exposed
		}
		// TODO(hap): create the accessory and bind characteristics.
		br.log.Info("publish accessory", "id", d.ID, "type", d.Type, "hap", accessoryKind(d.Type))
	}
	// TODO(hap): start hap.Server; on characteristic write -> br.bus.PublishCommand(...)
	<-ctx.Done()
	return ctx.Err()
}

// OnEvent reflects a device state change into the corresponding HAP characteristic.
func (br *Bridge) OnEvent(e domain.Event) {
	// TODO(hap): map e.State onto the accessory characteristic.
	br.log.Debug("homekit reflect", "id", e.DeviceID, "kind", e.Kind)
}
