// Package mqtt runs an embedded MQTT broker and bridges ESP32/ESPHome devices.
//
// TODO: back the broker with github.com/mochi-mqtt/server.
package mqtt

import (
	"context"
	"log/slog"

	"github.com/sh5080/home-hub/internal/bus"
	"github.com/sh5080/home-hub/internal/domain"
)

// Driver embeds an MQTT broker and maps topics to domain devices.
type Driver struct {
	listen string
	bus    *bus.Bus
	log    *slog.Logger
}

// New builds an MQTT driver whose embedded broker listens on the given address.
func New(listen string, b *bus.Bus, log *slog.Logger) *Driver {
	return &Driver{listen: listen, bus: b, log: log}
}

// Name identifies the adapter.
func (d *Driver) Name() string { return "mqtt" }

// Start runs the embedded broker until ctx is cancelled.
func (d *Driver) Start(ctx context.Context) error {
	d.log.Info("mqtt broker starting", "listen", d.listen)
	// TODO(mochi): start broker; on device publish -> d.bus.PublishEvent(...)
	<-ctx.Done()
	return ctx.Err()
}

// Apply publishes a set-command for the target device.
func (d *Driver) Apply(cmd domain.Command) error {
	// TODO(mochi): publish the device's set-topic.
	d.log.Info("mqtt apply", "device", cmd.DeviceID, "action", cmd.Action)
	return nil
}
