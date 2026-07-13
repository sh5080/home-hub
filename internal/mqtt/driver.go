// Package mqtt runs an embedded MQTT broker (mochi-mqtt) and bridges ESP32
// devices onto the bus.
//
// Topic convention (base = a device's configured addr, or "home/<id>" when
// addr is empty):
//
//	<base>/state  device → hub   JSON {"on":bool,"level":n,"value":x}
//	<base>/set    hub → device   JSON {"on":bool,"position":n,"level":n}
//
// An ESP32 subscribes to its own set-topic and reports on its state-topic.
// Fields are optional in both directions; a device reports what it has (a
// sensor only "value", an RF fan bridge "on"/"level").
package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	mochi "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"

	"github.com/sh5080/home-hub/internal/bus"
	"github.com/sh5080/home-hub/internal/domain"
	"github.com/sh5080/home-hub/internal/registry"
)

// statePayload is what devices publish on home/<id>/state.
type statePayload struct {
	On        *bool    `json:"on,omitempty"`
	Position  *int     `json:"position,omitempty"`
	Level     *int     `json:"level,omitempty"`
	Value     *float64 `json:"value,omitempty"`
	ColorTemp *int     `json:"colortemp,omitempty"`
	Hue       *float64 `json:"hue,omitempty"`
	Sat       *float64 `json:"sat,omitempty"`
	Direction *int     `json:"direction,omitempty"`
}

// setPayload is what the hub publishes on home/<id>/set.
type setPayload struct {
	On        *bool    `json:"on,omitempty"`
	Position  *int     `json:"position,omitempty"`
	Level     *int     `json:"level,omitempty"`
	ColorTemp *int     `json:"colortemp,omitempty"`
	Hue       *float64 `json:"hue,omitempty"`
	Sat       *float64 `json:"sat,omitempty"`
	Direction *int     `json:"direction,omitempty"`
}

// Driver embeds an MQTT broker and maps topics to domain devices.
type Driver struct {
	listen string
	bus    *bus.Bus
	reg    *registry.Registry
	log    *slog.Logger
	server *mochi.Server
}

// New builds an MQTT driver whose embedded broker listens on the given address.
func New(listen string, b *bus.Bus, reg *registry.Registry, log *slog.Logger) *Driver {
	server := mochi.New(&mochi.Options{InlineClient: true})
	// LAN-open broker: devices authenticate by being on the home network. If
	// the hub ever faces an untrusted network, replace with auth.Ledger.
	_ = server.AddHook(new(auth.AllowHook), nil)
	return &Driver{listen: listen, bus: b, reg: reg, log: log, server: server}
}

// Name identifies the adapter.
func (d *Driver) Name() string { return "mqtt" }

// topicBase is the device's MQTT topic prefix: its configured addr, or
// "home/<id>" when addr is empty. State arrives on "<base>/state" and commands
// go to "<base>/set".
func topicBase(dev domain.Device) string {
	if dev.Addr != "" {
		return dev.Addr
	}
	return "home/" + dev.ID
}

// Start runs the embedded broker until ctx is cancelled.
func (d *Driver) Start(ctx context.Context) error {
	if err := d.server.AddListener(listeners.NewTCP(listeners.Config{ID: "hub", Address: d.listen})); err != nil {
		return fmt.Errorf("mqtt listener: %w", err)
	}
	// Subscribe each MQTT device's state topic, binding its id into the handler
	// so the addr (topic base) is the source of truth rather than the id.
	sid := 1
	for _, dev := range d.reg.List() {
		if dev.Integration != domain.MQTT {
			continue
		}
		id := dev.ID
		if err := d.server.Subscribe(topicBase(dev)+"/state", sid, func(_ *mochi.Client, _ packets.Subscription, pk packets.Packet) {
			d.onState(id, pk)
		}); err != nil {
			return fmt.Errorf("mqtt subscribe %s: %w", id, err)
		}
		sid++
	}
	errCh := make(chan error, 1)
	go func() { errCh <- d.server.Serve() }()
	d.log.Info("mqtt broker listening", "addr", d.listen)

	select {
	case <-ctx.Done():
		_ = d.server.Close()
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

// onState maps a device's state publication (on the device's bound topic) onto
// a bus event.
func (d *Driver) onState(id string, pk packets.Packet) {
	var p statePayload
	if err := json.Unmarshal(pk.Payload, &p); err != nil {
		d.log.Warn("mqtt bad state payload", "topic", pk.TopicName, "err", err)
		return
	}
	if p.On == nil && p.Position == nil && p.Level == nil && p.Value == nil &&
		p.ColorTemp == nil && p.Hue == nil && p.Sat == nil && p.Direction == nil {
		return // nothing reported
	}
	d.bus.PublishEvent(domain.Event{
		DeviceID: id,
		Kind:     domain.EventStateChanged,
		State: domain.State{
			On: p.On, Position: p.Position, Level: p.Level, Value: p.Value,
			ColorTemp: p.ColorTemp, Hue: p.Hue, Sat: p.Sat, Direction: p.Direction,
		},
	})
}

// Apply publishes a set-command on the target device's set-topic.
func (d *Driver) Apply(cmd domain.Command) error {
	dev, ok := d.reg.Get(cmd.DeviceID)
	if !ok {
		return fmt.Errorf("mqtt: unknown device %s", cmd.DeviceID)
	}
	var p setPayload
	switch cmd.Action {
	case domain.ActionSetOn:
		on, _ := cmd.Value.(bool)
		p.On = &on
	case domain.ActionSetPosition:
		pos, _ := cmd.Value.(int)
		p.Position = &pos
	case domain.ActionSetLevel:
		lvl, _ := cmd.Value.(int)
		p.Level = &lvl
	case domain.ActionSetColorTemp:
		ct, _ := cmd.Value.(int)
		p.ColorTemp = &ct
	case domain.ActionSetColor:
		hs, _ := cmd.Value.(domain.HueSat)
		p.Hue = &hs.Hue
		p.Sat = &hs.Sat
	case domain.ActionSetDirection:
		dir, _ := cmd.Value.(int)
		p.Direction = &dir
	default:
		return nil
	}
	payload, err := json.Marshal(p)
	if err != nil {
		return err
	}
	topic := topicBase(dev) + "/set"
	d.log.Info("mqtt apply", "topic", topic, "payload", string(payload))
	// retain=false: commands are edge-triggered, not state.
	return d.server.Publish(topic, payload, false, 0)
}
