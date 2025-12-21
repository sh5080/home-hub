// Package homekit exposes hub devices to HomeKit controllers over HAP,
// backed by github.com/brutella/hap.
package homekit

import (
	"context"
	"log/slog"
	"time"

	"github.com/brutella/hap"
	"github.com/brutella/hap/accessory"

	"github.com/sh5080/home-hub/internal/bus"
	"github.com/sh5080/home-hub/internal/domain"
	"github.com/sh5080/home-hub/internal/registry"
)

// Config configures the HAP bridge exposed to HomeKit controllers.
type Config struct {
	Name    string
	Pin     string
	Addr    string // e.g. ":51826"
	Storage string
}

// Bridge publishes hub devices to HomeKit and forwards HomeKit-originated
// commands onto the bus. Virtual "trigger" switches (for Matter delegation)
// are also exposed here.
type Bridge struct {
	cfg Config
	bus *bus.Bus
	reg *registry.Registry
	log *slog.Logger

	devs     map[string]devAccessory
	triggers map[string]*accessory.Switch
}

// New builds a HomeKit bridge.
func New(cfg Config, b *bus.Bus, reg *registry.Registry, log *slog.Logger) *Bridge {
	return &Bridge{
		cfg: cfg, bus: b, reg: reg, log: log,
		devs:     make(map[string]devAccessory),
		triggers: make(map[string]*accessory.Switch),
	}
}

// Name identifies the adapter.
func (br *Bridge) Name() string { return "homekit" }

// RegisterTrigger exposes a stateless virtual switch that HomeKit automations
// can react to, and returns a func that "presses" it.
func (br *Bridge) RegisterTrigger(id string) func() {
	sw := accessory.NewSwitch(accessory.Info{Name: id, SerialNumber: id, Manufacturer: "home-hub"})
	br.triggers[id] = sw
	return func() {
		// Pulse On->Off so a HomeKit automation with a "turns on" trigger fires.
		sw.Switch.On.SetValue(true)
		time.AfterFunc(500*time.Millisecond, func() { sw.Switch.On.SetValue(false) })
		br.log.Info("virtual trigger pressed", "trigger", id)
	}
}

// Start builds accessories from the registry and serves HAP until ctx is done.
func (br *Bridge) Start(ctx context.Context) error {
	bridge := accessory.NewBridge(accessory.Info{Name: br.cfg.Name, Manufacturer: "home-hub"})

	var as []*accessory.A
	for _, d := range br.reg.List() {
		if d.Integration == domain.Matter {
			continue // delegated to HomeKit; only triggers are exposed
		}
		da := br.buildAccessory(d, br.bus.PublishCommand)
		br.devs[d.ID] = da
		as = append(as, da.a)
		br.log.Info("publish accessory", "id", d.ID, "type", d.Type)
	}
	for id, sw := range br.triggers {
		as = append(as, sw.A)
		br.log.Info("publish virtual trigger", "trigger", id)
	}

	store := hap.NewFsStore(br.cfg.Storage)
	server, err := hap.NewServer(store, bridge.A, as...)
	if err != nil {
		return err
	}
	server.Pin = br.cfg.Pin
	server.Addr = br.cfg.Addr
	br.log.Info("hap server listening", "name", br.cfg.Name, "addr", br.cfg.Addr)
	return server.ListenAndServe(ctx)
}

// OnEvent reflects a device state change onto its HAP characteristics so the
// change is pushed to HomeKit controllers.
func (br *Bridge) OnEvent(e domain.Event) {
	da, ok := br.devs[e.DeviceID]
	if !ok {
		return
	}
	da.apply(e.State)
	br.log.Debug("homekit reflect", "id", e.DeviceID, "kind", e.Kind)
}
