// Package zigbee controls Zigbee devices through a serial coordinator.
//
// TODO: back this with github.com/shimmeringbee (zstack + zcl) against a
// CC2652-based coordinator (e.g. Sonoff ZBDongle-P). Vendor quirks are isolated
// in quirks.go.
package zigbee

import (
	"context"
	"log/slog"

	"github.com/sh5080/home-hub/internal/bus"
	"github.com/sh5080/home-hub/internal/domain"
)

// Driver is the Zigbee protocol adapter.
type Driver struct {
	port string
	bus  *bus.Bus
	log  *slog.Logger
}

// New builds a Zigbee driver bound to the given serial coordinator port.
func New(port string, b *bus.Bus, log *slog.Logger) *Driver {
	return &Driver{port: port, bus: b, log: log}
}

// Name identifies the adapter.
func (d *Driver) Name() string { return "zigbee" }

// Start opens the coordinator and processes attribute reports until ctx is done.
func (d *Driver) Start(ctx context.Context) error {
	d.log.Info("zigbee starting", "port", d.port)
	// TODO(zstack): open serial, form/resume the network, subscribe to reports,
	//   translate reports -> d.bus.PublishEvent(domain.Event{...})
	<-ctx.Done()
	return ctx.Err()
}

// Apply sends a command to a Zigbee device.
func (d *Driver) Apply(cmd domain.Command) error {
	// TODO(zcl): map cmd -> ZCL cluster command (On/Off, Level Control) and send.
	d.log.Info("zigbee apply", "device", cmd.DeviceID, "action", cmd.Action)
	return nil
}
