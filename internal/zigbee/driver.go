// Package zigbee controls Zigbee devices through a serial coordinator, backed
// by shimmeringbee zstack (e.g. a CC2652-based Sonoff ZBDongle-P). Vendor
// quirks are isolated in quirks.go.
package zigbee

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/shimmeringbee/persistence"
	"github.com/shimmeringbee/persistence/impl/file"
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
	baudRate     = 115200

	onOffCluster      = zigbee.ClusterID(0x0006)
	covCluster        = zigbee.ClusterID(0x0102) // Window Covering (curtain motors)
	multistateCluster = zigbee.ClusterID(0x0012) // Multistate Input (Aqara decoupled buttons)
	lumiCluster       = zigbee.ClusterID(0xFCC0) // Aqara/Xiaomi manufacturer-specific
	lumiMfgCode       = uint16(0x115F)           // Xiaomi/Lumi manufacturer code
	lumiAttrOpMode    = uint16(0x0200)           // per-endpoint switch operation mode

	// ZCL Window Covering commands (same ids Matter later inherited).
	covCmdUpOpen    = 0x00
	covCmdDownClose = 0x01
	covCmdStop      = 0x02
	covCmdGoToLift  = 0x05 // payload: lift percentage uint8, 0=open 100=closed

	covAttrLiftPercentage = uint16(0x0008) // CurrentPositionLiftPercentage, uint8
)

// Config configures the Zigbee adapter.
type Config struct {
	Port string
	// Storage is a directory for coordinator + network persistence. The network
	// configuration (incl. the network key) is created once and reused across
	// restarts so paired devices stay paired. Empty = volatile in-memory mode:
	// every restart forms a NEW network and all devices must be re-paired —
	// acceptable only for development.
	Storage string
	// PermitJoin opens the network for new devices. Keep it off in normal
	// operation and enable it only for pairing sessions: an open Zigbee network
	// accepts any join request.
	PermitJoin bool
}

// Driver is the Zigbee protocol adapter.
type Driver struct {
	cfg Config
	bus *bus.Bus
	reg *registry.Registry
	log *slog.Logger

	z   *zstack.ZStack
	seq uint8
}

// New builds a Zigbee driver bound to the given serial coordinator port.
func New(cfg Config, b *bus.Bus, reg *registry.Registry, log *slog.Logger) *Driver {
	return &Driver{cfg: cfg, bus: b, reg: reg, log: log}
}

// Name identifies the adapter.
func (d *Driver) Name() string { return "zigbee" }

// networkConfig loads the persisted network configuration, generating and
// saving one on first run. With no storage dir it always generates (dev mode).
func (d *Driver) networkConfig() (zigbee.NetworkConfiguration, error) {
	if d.cfg.Storage == "" {
		d.log.Warn("zigbee running without storage: devices must re-pair after every restart")
		return zigbee.GenerateNetworkConfiguration()
	}
	path := filepath.Join(d.cfg.Storage, "network.json")
	if raw, err := os.ReadFile(path); err == nil {
		var nc zigbee.NetworkConfiguration
		if err := json.Unmarshal(raw, &nc); err != nil {
			return nc, fmt.Errorf("parse %s: %w", path, err)
		}
		return nc, nil
	}
	nc, err := zigbee.GenerateNetworkConfiguration()
	if err != nil {
		return nc, err
	}
	raw, err := json.MarshalIndent(nc, "", "  ")
	if err != nil {
		return nc, err
	}
	if err := os.MkdirAll(d.cfg.Storage, 0o700); err != nil {
		return nc, err
	}
	// 0600: the file contains the Zigbee network key.
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return nc, err
	}
	d.log.Info("zigbee network configuration created", "path", path)
	return nc, nil
}

// Start opens the coordinator, initialises the network, and registers the
// adapter endpoint, then blocks until ctx is cancelled.
func (d *Driver) Start(ctx context.Context) error {
	port, err := serial.Open(d.cfg.Port, &serial.Mode{BaudRate: baudRate})
	if err != nil {
		return fmt.Errorf("open serial %s: %w", d.cfg.Port, err)
	}
	defer port.Close()

	var store persistence.Section
	if d.cfg.Storage != "" {
		if err := os.MkdirAll(d.cfg.Storage, 0o700); err != nil {
			return fmt.Errorf("zigbee storage: %w", err)
		}
		store = file.New(filepath.Join(d.cfg.Storage, "zstack"))
	} else {
		store = memory.New()
	}
	d.z = zstack.New(port, store)
	defer d.z.Stop()

	nc, err := d.networkConfig()
	if err != nil {
		return fmt.Errorf("network config: %w", err)
	}
	if err := d.z.Initialise(ctx, nc); err != nil {
		return fmt.Errorf("initialise coordinator: %w", err)
	}
	if err := d.z.RegisterAdapterEndpoint(ctx, adapterEndpt, haProfile, 0, 0,
		[]zigbee.ClusterID{onOffCluster, covCluster, multistateCluster},
		[]zigbee.ClusterID{onOffCluster, covCluster, multistateCluster}); err != nil {
		return fmt.Errorf("register adapter endpoint: %w", err)
	}
	d.log.Info("zigbee coordinator initialised", "port", d.cfg.Port, "storage", d.cfg.Storage)

	if d.cfg.PermitJoin {
		if err := d.z.PermitJoin(ctx, true); err != nil {
			d.log.Warn("permit join failed", "err", err)
		} else {
			d.log.Warn("zigbee network OPEN for joining; disable permitJoin after pairing")
		}
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
			d.configureJoined(ctx, e.IEEEAddress)
		case zigbee.NodeIncomingMessageEvent:
			d.handleIncoming(e)
		default:
			d.log.Debug("zigbee event", "type", fmt.Sprintf("%T", event))
		}
	}
}

// deviceEndpoint is the device's ZCL endpoint: gang N of a multi-gang switch,
// defaulting to endpoint 1.
func deviceEndpoint(dev domain.Device) zigbee.Endpoint {
	if dev.Endpoint == 0 {
		return zigbee.Endpoint(1)
	}
	return zigbee.Endpoint(dev.Endpoint)
}

// lumiDecoupledMode is the operation-mode value that detaches the relay from
// the paddle (Aqara "decoupled"). control-relay (coupled) is 0x01.
//
// NOTE: value confirmed against zigbee2mqtt's Aqara wall-switch converters and
// stable across that line; verify on the H2 specifically, since it is newer —
// if a press still toggles the load, this enum is the thing to check.
const lumiDecoupledMode = byte(0x00)

// lumiOpModeFrame builds a manufacturer-specific ZCL Write Attributes frame
// setting the Aqara operation-mode attribute (0xFCC0/0x0200, uint8).
//
// Frame: FCF(0x04 = manufacturer-specific, general command) | mfgCode(LE) |
// seq | cmd(0x02 Write Attributes) | attrID(LE) | type(0x20) | value.
func lumiOpModeFrame(seq, value byte) []byte {
	mfg := uint16(lumiMfgCode)
	attr := uint16(lumiAttrOpMode)
	return []byte{
		0x04,                      // frame control: manufacturer-specific
		byte(mfg), byte(mfg >> 8), // manufacturer code (LE)
		seq,
		0x02,                        // Write Attributes
		byte(attr), byte(attr >> 8), // attribute id (LE)
		0x20,  // type: uint8
		value, // operation mode
	}
}

// configureJoined applies per-device provisioning after a node joins: for now,
// decoupled mode on Aqara switches that request it. Best-effort — a failure is
// logged, not fatal; the user can re-pair.
func (d *Driver) configureJoined(ctx context.Context, addr zigbee.IEEEAddress) {
	if !isLumi(addr) {
		return
	}
	for _, dev := range d.reg.List() {
		if dev.Integration != domain.Zigbee || !dev.Decoupled {
			continue
		}
		if a, err := parseIEEE(dev.Addr); err != nil || a != addr {
			continue
		}
		d.seq++
		msg := zigbee.ApplicationMessage{
			ClusterID:           lumiCluster,
			SourceEndpoint:      adapterEndpt,
			DestinationEndpoint: deviceEndpoint(dev),
			Data:                lumiOpModeFrame(d.seq, lumiDecoupledMode),
		}
		wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := d.z.SendApplicationMessageToNode(wctx, addr, msg, true)
		cancel()
		if err != nil {
			d.log.Warn("aqara decoupled config failed", "device", dev.ID, "err", err)
			continue
		}
		d.log.Info("aqara decoupled mode set", "device", dev.ID, "endpoint", deviceEndpoint(dev))
	}
}

// Apply maps a domain command onto a ZCL cluster command for the device.
func (d *Driver) Apply(cmd domain.Command) error {
	if d.z == nil {
		return fmt.Errorf("zigbee coordinator not ready")
	}
	dev, ok := d.reg.Get(cmd.DeviceID)
	if !ok {
		return fmt.Errorf("unknown device %s", cmd.DeviceID)
	}
	addr, err := parseIEEE(dev.Addr)
	if err != nil {
		return err
	}

	var cluster zigbee.ClusterID
	var frame []byte
	switch cmd.Action {
	case domain.ActionSetOn:
		on, _ := cmd.Value.(bool)
		if dev.Type == domain.TypeCover {
			// On/off against a curtain motor means open/close.
			cluster, frame = covCluster, d.zclCommand(covCmdDownClose)
			if on {
				cluster, frame = covCluster, d.zclCommand(covCmdUpOpen)
			}
			break
		}
		cluster, frame = onOffCluster, d.zclCommand(0x00) // Off
		if on {
			cluster, frame = onOffCluster, d.zclCommand(0x01) // On
		}
	case domain.ActionSetPosition:
		p, _ := cmd.Value.(int)
		if p < 0 {
			p = 0
		}
		if p > 100 {
			p = 100
		}
		// Domain position is percent-open; ZCL lift percentage is percent-closed.
		cluster, frame = covCluster, d.zclCommand(covCmdGoToLift, byte(100-p))
	default:
		return nil
	}

	msg := zigbee.ApplicationMessage{
		ClusterID:           cluster,
		SourceEndpoint:      adapterEndpt,
		DestinationEndpoint: deviceEndpoint(dev),
		Data:                frame,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return d.z.SendApplicationMessageToNode(ctx, addr, msg, true)
}

// zclCommand builds a cluster-specific ZCL frame: frame control (0x01),
// transaction seq, command id, then any payload bytes.
func (d *Driver) zclCommand(commandID byte, payload ...byte) []byte {
	d.seq++
	return append([]byte{0x01, d.seq, commandID}, payload...)
}

// parseIEEE converts a "0x..." hex string into a Zigbee IEEE address.
func parseIEEE(s string) (zigbee.IEEEAddress, error) {
	v, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(s), "0x"), 16, 64)
	if err != nil {
		return 0, fmt.Errorf("bad ieee address %q: %w", s, err)
	}
	return zigbee.IEEEAddress(v), nil
}

// handleIncoming turns device reports into bus events: On/Off state, curtain
// position (Window Covering), and Aqara decoupled-button presses (Multistate).
func (d *Driver) handleIncoming(e zigbee.NodeIncomingMessageEvent) {
	msg := e.IncomingMessage.ApplicationMessage
	id := d.deviceIDByAddr(e.IEEEAddress, msg.SourceEndpoint)
	if id == "" {
		return
	}

	switch msg.ClusterID {
	case onOffCluster:
		on, ok := parseAttrBool(msg.Data, 0x0000)
		if !ok && isLumi(e.IEEEAddress) {
			on, ok = aqaraOnOff(msg.Data) // Xiaomi manufacturer-specific report
		}
		if !ok {
			return
		}
		d.bus.PublishEvent(domain.Event{
			DeviceID: id,
			Kind:     domain.EventStateChanged,
			State:    domain.State{On: domain.BoolPtr(on)},
		})

	case covCluster:
		zclPct, ok := parseAttrUint8(msg.Data, covAttrLiftPercentage)
		if !ok {
			return
		}
		// ZCL lift percentage is percent-closed; domain position is percent-open.
		pos := 100 - int(zclPct)
		d.bus.PublishEvent(domain.Event{
			DeviceID: id,
			Kind:     domain.EventStateChanged,
			State:    domain.State{Position: domain.IntPtr(pos)},
		})

	case multistateCluster:
		press, ok := parseMultistatePress(msg.Data)
		if !ok {
			return
		}
		d.log.Info("zigbee button", "device", id, "press", press)
		d.bus.PublishEvent(domain.Event{
			DeviceID: id,
			Kind:     domain.EventButton,
			Press:    press,
		})
	}
}

// zclReportRecord returns the first attribute record of a ZCL Report Attributes
// (0x0a) frame: attrID(2 LE) | dataType(1) | value... Best-effort.
func zclReportRecord(data []byte) ([]byte, bool) {
	if len(data) < 3 || data[2] != 0x0a { // command 0x0a = Report Attributes
		return nil, false
	}
	rec := data[3:]
	if len(rec) < 4 {
		return nil, false
	}
	return rec, true
}

// parseAttrBool extracts a boolean attribute report for attrID.
func parseAttrBool(data []byte, attrID uint16) (bool, bool) {
	rec, ok := zclReportRecord(data)
	if !ok {
		return false, false
	}
	id := uint16(rec[0]) | uint16(rec[1])<<8
	if id != attrID || rec[2] != 0x10 { // 0x10 = boolean
		return false, false
	}
	return rec[3] != 0x00, true
}

// parseAttrUint8 extracts a uint8 attribute report for attrID.
func parseAttrUint8(data []byte, attrID uint16) (uint8, bool) {
	rec, ok := zclReportRecord(data)
	if !ok {
		return 0, false
	}
	id := uint16(rec[0]) | uint16(rec[1])<<8
	if id != attrID || rec[2] != 0x20 { // 0x20 = uint8
		return 0, false
	}
	return rec[3], true
}

// parseMultistatePress maps a Multistate Input PresentValue (attr 0x0055)
// report to a press kind. Aqara switches in decoupled mode report 1=single,
// 2=double, 0=hold on this cluster.
//
// NOTE: validated against zigbee2mqtt/zigbee-herdsman converters for Aqara
// wall switches; confirm the exact values for the H2 on real hardware (they
// have been stable across the Aqara wall-switch line, but H2 is new).
func parseMultistatePress(data []byte) (string, bool) {
	rec, ok := zclReportRecord(data)
	if !ok {
		return "", false
	}
	id := uint16(rec[0]) | uint16(rec[1])<<8
	if id != 0x0055 || rec[2] != 0x21 { // PresentValue, type 0x21 = uint16
		return "", false
	}
	if len(rec) < 5 {
		return "", false
	}
	switch uint16(rec[3]) | uint16(rec[4])<<8 {
	case 0:
		return domain.PressHold, true
	case 1:
		return domain.PressSingle, true
	case 2:
		return domain.PressDouble, true
	default:
		return "", false
	}
}

// deviceIDByAddr resolves a Zigbee IEEE address + source endpoint back to a
// configured device id, so each gang of a multi-gang switch maps to its own
// device. A device with no explicit endpoint matches endpoint 1.
func (d *Driver) deviceIDByAddr(addr zigbee.IEEEAddress, endpoint zigbee.Endpoint) string {
	fallback := "" // same address, endpoint not modeled: single-unit device
	for _, dev := range d.reg.List() {
		if dev.Integration != domain.Zigbee {
			continue
		}
		a, err := parseIEEE(dev.Addr)
		if err != nil || a != addr {
			continue
		}
		if deviceEndpoint(dev) == endpoint {
			return dev.ID
		}
		if dev.Endpoint == 0 {
			fallback = dev.ID
		}
	}
	return fallback
}
