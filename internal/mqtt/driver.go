// Package mqtt runs an embedded MQTT broker (mochi-mqtt) and bridges ESP32
// devices onto the bus.
//
// Topic convention (deviceID = the id from devices.yaml):
//
//	home/<deviceID>/state  device → hub   JSON {"on":bool,"level":n,"value":x}
//	home/<deviceID>/set    hub → device   JSON {"on":bool,"position":n,"level":n}
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
	"strings"

	mochi "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"

	"github.com/sh5080/home-hub/internal/bus"
	"github.com/sh5080/home-hub/internal/domain"
)

// statePayload is what devices publish on home/<id>/state.
type statePayload struct {
	On       *bool    `json:"on,omitempty"`
	Position *int     `json:"position,omitempty"`
	Level    *int     `json:"level,omitempty"`
	Value    *float64 `json:"value,omitempty"`
}

// setPayload is what the hub publishes on home/<id>/set.
type setPayload struct {
	On       *bool `json:"on,omitempty"`
	Position *int  `json:"position,omitempty"`
	Level    *int  `json:"level,omitempty"`
}

// Driver embeds an MQTT broker and maps topics to domain devices.
type Driver struct {
	listen string
	bus    *bus.Bus
	log    *slog.Logger
	server *mochi.Server
}

// New builds an MQTT driver whose embedded broker listens on the given address.
func New(listen string, b *bus.Bus, log *slog.Logger) *Driver {
	server := mochi.New(&mochi.Options{InlineClient: true})
	// LAN-open broker: devices authenticate by being on the home network. If
	// the hub ever faces an untrusted network, replace with auth.Ledger.
	_ = server.AddHook(new(auth.AllowHook), nil)
	return &Driver{listen: listen, bus: b, log: log, server: server}
}

// Name identifies the adapter.
func (d *Driver) Name() string { return "mqtt" }

// Start runs the embedded broker until ctx is cancelled.
func (d *Driver) Start(ctx context.Context) error {
	if err := d.server.AddListener(listeners.NewTCP(listeners.Config{ID: "hub", Address: d.listen})); err != nil {
		return fmt.Errorf("mqtt listener: %w", err)
	}
	if err := d.server.Subscribe("home/+/state", 1, d.onState); err != nil {
		return fmt.Errorf("mqtt subscribe: %w", err)
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

// onState maps a device's state publication onto a bus event.
func (d *Driver) onState(_ *mochi.Client, _ packets.Subscription, pk packets.Packet) {
	id, ok := deviceIDFromTopic(pk.TopicName)
	if !ok {
		return
	}
	var p statePayload
	if err := json.Unmarshal(pk.Payload, &p); err != nil {
		d.log.Warn("mqtt bad state payload", "topic", pk.TopicName, "err", err)
		return
	}
	if p.On == nil && p.Position == nil && p.Level == nil && p.Value == nil {
		return // nothing reported
	}
	d.bus.PublishEvent(domain.Event{
		DeviceID: id,
		Kind:     domain.EventStateChanged,
		State:    domain.State{On: p.On, Position: p.Position, Level: p.Level, Value: p.Value},
	})
}

// deviceIDFromTopic extracts <id> from home/<id>/state.
func deviceIDFromTopic(topic string) (string, bool) {
	parts := strings.Split(topic, "/")
	if len(parts) != 3 || parts[0] != "home" || parts[2] != "state" || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

// Apply publishes a set-command on the target device's set-topic.
func (d *Driver) Apply(cmd domain.Command) error {
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
	default:
		return nil
	}
	payload, err := json.Marshal(p)
	if err != nil {
		return err
	}
	topic := "home/" + cmd.DeviceID + "/set"
	d.log.Info("mqtt apply", "topic", topic, "payload", string(payload))
	// retain=false: commands are edge-triggered, not state.
	return d.server.Publish(topic, payload, false, 0)
}
