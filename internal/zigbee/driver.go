// Package zigbee controls Zigbee devices through a serial coordinator, backed
// by shimmeringbee zstack (e.g. a CC2652-based Sonoff ZBDongle-P). Vendor
// quirks are isolated in quirks.go.
package zigbee

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

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
		case zigbee.NodeIncomingMessageEvent:
			d.handleIncoming(e)
		default:
			d.log.Debug("zigbee event", "type", fmt.Sprintf("%T", event))
		}
	}
}

// Apply maps a command to a ZCL On/Off cluster command and sends it.
func (d *Driver) Apply(cmd domain.Command) error {
	if d.z == nil {
		return fmt.Errorf("zigbee coordinator not ready")
	}
	if cmd.Action != domain.ActionSetOn {
		return nil
	}
	dev, ok := d.reg.Get(cmd.DeviceID)
	if !ok {
		return fmt.Errorf("unknown device %s", cmd.DeviceID)
	}
	addr, err := parseIEEE(dev.Addr)
	if err != nil {
		return err
	}

	on, _ := cmd.Value.(bool)
	var commandID byte = 0x00 // Off
	if on {
		commandID = 0x01 // On
	}
	d.seq++
	// ZCL cluster-specific frame: frame control (0x01), transaction seq, command id.
	msg := zigbee.ApplicationMessage{
		ClusterID:           onOffCluster,
		SourceEndpoint:      adapterEndpt,
		DestinationEndpoint: adapterEndpt,
		Data:                []byte{0x01, d.seq, commandID},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return d.z.SendApplicationMessageToNode(ctx, addr, msg, true)
}

// parseIEEE converts a "0x..." hex string into a Zigbee IEEE address.
func parseIEEE(s string) (zigbee.IEEEAddress, error) {
	v, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(s), "0x"), 16, 64)
	if err != nil {
		return 0, fmt.Errorf("bad ieee address %q: %w", s, err)
	}
	return zigbee.IEEEAddress(v), nil
}

// handleIncoming turns an On/Off attribute report into a state event.
func (d *Driver) handleIncoming(e zigbee.NodeIncomingMessageEvent) {
	msg := e.IncomingMessage.ApplicationMessage
	if msg.ClusterID != onOffCluster {
		return
	}
	on, ok := parseOnOffReport(msg.Data)
	if !ok && isLumi(e.IEEEAddress) {
		on, ok = aqaraOnOff(msg.Data) // Xiaomi/Aqara manufacturer-specific report
	}
	if !ok {
		return
	}
	id := d.deviceIDByAddr(e.IEEEAddress)
	if id == "" {
		return
	}
	d.bus.PublishEvent(domain.Event{
		DeviceID: id,
		Kind:     domain.EventStateChanged,
		State:    domain.State{On: domain.BoolPtr(on)},
	})
}

// parseOnOffReport extracts the On/Off value from a ZCL Report Attributes
// (0x0a) frame for the On/Off cluster attribute 0x0000 (boolean). Best-effort.
func parseOnOffReport(data []byte) (bool, bool) {
	if len(data) < 3 || data[2] != 0x0a { // command 0x0a = Report Attributes
		return false, false
	}
	rec := data[3:]
	if len(rec) < 4 {
		return false, false
	}
	attrID := uint16(rec[0]) | uint16(rec[1])<<8
	dataType := rec[2]
	if attrID != 0x0000 || dataType != 0x10 { // 0x10 = boolean
		return false, false
	}
	return rec[3] != 0x00, true
}

// deviceIDByAddr resolves a Zigbee IEEE address back to a configured device id.
func (d *Driver) deviceIDByAddr(addr zigbee.IEEEAddress) string {
	for _, dev := range d.reg.List() {
		if dev.Integration != domain.Zigbee {
			continue
		}
		if a, err := parseIEEE(dev.Addr); err == nil && a == addr {
			return dev.ID
		}
	}
	return ""
}
