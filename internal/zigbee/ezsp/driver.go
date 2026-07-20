package ezsp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sh5080/home-hub/internal/bus"
	"github.com/sh5080/home-hub/internal/domain"
	"github.com/sh5080/home-hub/internal/registry"
	"github.com/sh5080/home-hub/internal/zigbee/ezsp/ash"
)

// Config configures the EZSP ("E" dongle) Zigbee backend. It mirrors the
// zstack backend's config so the two are interchangeable via config selection.
type Config struct {
	Port       string
	Storage    string // network/key persistence (network form on first run)
	PermitJoin bool
}

// Driver is the EZSP Zigbee adapter. It implements driver.Driver alongside the
// zstack driver; the server picks one via config `zigbee.backend`.
//
// STATUS: M0 scaffold — connects to the NCP and negotiates the EZSP version.
// TODO (next milestones): extended (v8) frame format; networkInit/formNetwork
// with persisted key; register endpoint + clusters; permitJoining; sendUnicast
// for ZCL on/off + window covering; decode incomingMessageHandler into bus
// events (on/off, cover position, Aqara multistate) mirroring the zstack driver.
type Driver struct {
	cfg  Config
	bus  *bus.Bus
	reg  *registry.Registry
	log  *slog.Logger
	conn *ash.Conn
	seq  uint8
}

// New builds an EZSP Zigbee driver.
func New(cfg Config, b *bus.Bus, reg *registry.Registry, log *slog.Logger) *Driver {
	return &Driver{cfg: cfg, bus: b, reg: reg, log: log}
}

// Name identifies the adapter. It reports "zigbee" so command routing (which
// keys on the domain integration) is identical to the zstack backend.
func (d *Driver) Name() string { return "zigbee" }

// nextSeq returns the next EZSP frame sequence number.
func (d *Driver) nextSeq() uint8 { d.seq++; return d.seq }

// Start opens the NCP, resets the ASH link, and negotiates the EZSP version
// (M0). The remaining coordinator bring-up is the next milestone; until then
// Start blocks reading callbacks so a misbehaving NCP is visible in logs.
func (d *Driver) Start(ctx context.Context) error {
	conn, err := ash.Open(d.cfg.Port, func(msg string, kv ...any) { d.log.Debug("ash: "+msg, kv...) })
	if err != nil {
		return err
	}
	d.conn = conn
	defer conn.Close()

	if err := conn.Reset(ctx); err != nil {
		return fmt.Errorf("ezsp reset: %w", err)
	}
	ver, err := d.negotiateVersion(ctx, 13)
	if err != nil {
		return fmt.Errorf("ezsp version: %w", err)
	}
	d.log.Info("ezsp NCP connected",
		"port", d.cfg.Port,
		"ezspVersion", ver.ProtocolVersion,
		"stackType", ver.StackType,
		"stackVersion", fmt.Sprintf("%#04x", ver.StackVersion))

	// TODO(M1+): networkInit → (formNetwork if none) → registerEndpoint →
	// permitJoining(cfg.PermitJoin). For now, surface callbacks in the log.
	d.log.Warn("ezsp backend is M0 (version handshake only); network + ZCL not yet implemented")
	return d.readCallbacks(ctx)
}

// negotiateVersion sends the version command and decodes the reply.
func (d *Driver) negotiateVersion(ctx context.Context, desired uint8) (VersionResponse, error) {
	if err := d.conn.Send(ctx, VersionCommand(d.nextSeq(), desired)); err != nil {
		return VersionResponse{}, err
	}
	resp, err := d.conn.Recv(ctx)
	if err != nil {
		return VersionResponse{}, err
	}
	return DecodeVersionResponse(resp)
}

// readCallbacks drains NCP-initiated frames. TODO: decode incomingMessage /
// trustCenterJoin / stackStatus into bus events like the zstack driver.
func (d *Driver) readCallbacks(ctx context.Context) error {
	for {
		p, err := d.conn.Recv(ctx)
		if err != nil {
			return err
		}
		d.log.Debug("ezsp callback frame", "bytes", fmt.Sprintf("% X", p))
	}
}

// Apply maps a domain command onto an EZSP unicast of a ZCL frame.
// TODO(M2): build the ZCL frame (reuse the zstack driver's on/off + window
// covering encoding) and send it via sendUnicast to the device's node id.
func (d *Driver) Apply(cmd domain.Command) error {
	return fmt.Errorf("ezsp: Apply not yet implemented (M0 scaffold) for %s", cmd.DeviceID)
}
