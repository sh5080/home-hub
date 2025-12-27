// Package zigbee controls Zigbee devices through a serial coordinator, backed
// by shimmeringbee zstack (e.g. a CC2652-based Sonoff ZBDongle-P). Vendor
// quirks are isolated in quirks.go.
package zigbee

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/shimmeringbee/persistence/impl/memory"
	"github.com/shimmeringbee/zigbee"
	"github.com/shimmeringbee/zstack"
	"go.bug.st/serial"

	"github.com/sh5080/home-hub/internal/bus"
	"github.com/sh5080/home-hub/internal/domain"
	"github.com/sh5080/home-hub/internal/registry"
)

const (
	haProfile    = zigbee.ProfileID(0x0104) // Home Automation
	adapterEndpt = zigbee.Endpoint(1)
	onOffCluster = zigbee.ClusterID(0x0006)
	baudRate     = 115200
)

// Driver is the Zigbee protocol adapter.
type Driver struct {
	port string
	bus  *bus.Bus
	reg  *registry.Registry
	log  *slog.Logger

	z   *zstack.ZStack
	seq uint8
}

// New builds a Zigbee driver bound to the given serial coordinator port.
func New(port string, b *bus.Bus, reg *registry.Registry, log *slog.Logger) *Driver {
	return &Driver{port: port, bus: b, reg: reg, log: log}
}

// Name identifies the adapter.
func (d *Driver) Name() string { return "zigbee" }

// Start opens the coordinator, initialises the network, and registers the
// adapter endpoint, then blocks until ctx is cancelled.
//
// NOTE: uses in-memory persistence + a freshly generated network here; a real
// deployment should use file persistence so the network survives restarts.
func (d *Driver) Start(ctx context.Context) error {
	port, err := serial.Open(d.port, &serial.Mode{BaudRate: baudRate})
	if err != nil {
		return fmt.Errorf("open serial %s: %w", d.port, err)
	}
	defer port.Close()

	d.z = zstack.New(port, memory.New())
	defer d.z.Stop()

	nc, err := zigbee.GenerateNetworkConfiguration()
	if err != nil {
		return fmt.Errorf("network config: %w", err)
	}
	if err := d.z.Initialise(ctx, nc); err != nil {
		return fmt.Errorf("initialise coordinator: %w", err)
	}
	if err := d.z.RegisterAdapterEndpoint(ctx, adapterEndpt, haProfile, 0, 0,
		[]zigbee.ClusterID{onOffCluster}, []zigbee.ClusterID{onOffCluster}); err != nil {
		return fmt.Errorf("register adapter endpoint: %w", err)
	}
	d.log.Info("zigbee coordinator initialised", "port", d.port)

	if err := d.z.PermitJoin(ctx, true); err != nil {
		d.log.Warn("permit join failed", "err", err)
	}
	return d.readEvents(ctx)
}

// readEvents consumes coordinator events until ctx is cancelled.
func (d *Driver) readEvents(ctx context.Context) error {
	for {
		event, err := d.z.ReadEvent(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			d.log.Error("zigbee read event", "err", err)
			continue
		}
		switch e := event.(type) {
		case zigbee.NodeJoinEvent:
			d.log.Info("zigbee node joined", "ieee", e.IEEEAddress.String())
		default:
			d.log.Debug("zigbee event", "type", fmt.Sprintf("%T", event))
		}
	}
}

// Apply sends a command to a Zigbee device.
func (d *Driver) Apply(cmd domain.Command) error {
	// TODO(12/28): map to a ZCL command and send via the coordinator.
	d.log.Info("zigbee apply", "device", cmd.DeviceID, "action", cmd.Action)
	return nil
}
