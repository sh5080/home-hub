package mqtt

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	mochi "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/packets"

	"github.com/sh5080/home-hub/internal/bus"
	"github.com/sh5080/home-hub/internal/domain"
)

func TestDeviceIDFromTopic(t *testing.T) {
	cases := []struct {
		topic string
		id    string
		ok    bool
	}{
		{"home/fan1/state", "fan1", true},
		{"home/fan1/set", "", false},
		{"other/fan1/state", "", false},
		{"home//state", "", false},
		{"home/fan1/state/extra", "", false},
	}
	for _, c := range cases {
		id, ok := deviceIDFromTopic(c.topic)
		if id != c.id || ok != c.ok {
			t.Fatalf("deviceIDFromTopic(%q) = %q,%v want %q,%v", c.topic, id, ok, c.id, c.ok)
		}
	}
}

// startDriver runs the broker on an ephemeral port and waits until it serves.
func startDriver(t *testing.T, b *bus.Bus) (*Driver, context.CancelFunc) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New("127.0.0.1:0", b, log)
	ctx, cancel := context.WithCancel(context.Background())
	go d.Start(ctx)
	time.Sleep(100 * time.Millisecond) // listener + inline subscription up
	return d, cancel
}

func TestStatePublicationBecomesEvent(t *testing.T) {
	b := bus.New(8)
	events := b.SubscribeEvents()
	d, cancel := startDriver(t, b)
	defer cancel()

	// A device (here: injected inline, as an ESP32 would over TCP) reports state.
	payload, _ := json.Marshal(statePayload{On: boolPtr(true), Value: floatPtr(23.5)})
	if err := d.server.Publish("home/fan1/state", payload, false, 0); err != nil {
		t.Fatal(err)
	}

	select {
	case e := <-events:
		if e.DeviceID != "fan1" || e.Kind != domain.EventStateChanged {
			t.Fatalf("event = %+v", e)
		}
		if e.State.On == nil || !*e.State.On || e.State.Value == nil || *e.State.Value != 23.5 {
			t.Fatalf("state = %+v", e.State)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event from state publication")
	}
}

func TestApplyPublishesSetTopic(t *testing.T) {
	b := bus.New(8)
	d, cancel := startDriver(t, b)
	defer cancel()

	got := make(chan packets.Packet, 1)
	if err := d.server.Subscribe("home/fan1/set", 99, func(_ *mochi.Client, _ packets.Subscription, pk packets.Packet) {
		got <- pk
	}); err != nil {
		t.Fatal(err)
	}

	if err := d.Apply(domain.SetLevel("fan1", 66)); err != nil {
		t.Fatal(err)
	}
	select {
	case pk := <-got:
		var p setPayload
		if err := json.Unmarshal(pk.Payload, &p); err != nil {
			t.Fatal(err)
		}
		if p.Level == nil || *p.Level != 66 || p.On != nil {
			t.Fatalf("payload = %s", pk.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no publication on set topic")
	}

	// Unknown action publishes nothing and does not error.
	if err := d.Apply(domain.Command{DeviceID: "fan1", Action: "bogus"}); err != nil {
		t.Fatal(err)
	}
}

func boolPtr(b bool) *bool        { return &b }
func floatPtr(f float64) *float64 { return &f }
