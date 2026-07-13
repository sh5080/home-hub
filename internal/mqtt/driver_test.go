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
	"github.com/sh5080/home-hub/internal/registry"
)

func TestTopicBase(t *testing.T) {
	// addr wins when set...
	if got := topicBase(domain.Device{ID: "fan1", Addr: "esp32/living/fan"}); got != "esp32/living/fan" {
		t.Fatalf("addr base = %q", got)
	}
	// ...and defaults to home/<id> otherwise.
	if got := topicBase(domain.Device{ID: "fan1"}); got != "home/fan1" {
		t.Fatalf("default base = %q", got)
	}
}

// startDriver registers devs, runs the broker on an ephemeral port, and waits
// until it serves.
func startDriver(t *testing.T, b *bus.Bus, devs ...domain.Device) (*Driver, context.CancelFunc) {
	t.Helper()
	reg := registry.New()
	for _, dv := range devs {
		reg.Add(dv)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New("127.0.0.1:0", b, reg, log)
	ctx, cancel := context.WithCancel(context.Background())
	go d.Start(ctx)
	time.Sleep(150 * time.Millisecond) // listener + per-device subscriptions up
	return d, cancel
}

func TestStatePublicationBecomesEvent(t *testing.T) {
	b := bus.New(8)
	events := b.SubscribeEvents()
	// The device's addr sets its topic base — deliberately not home/<id>.
	dev := domain.Device{ID: "fan1", Integration: domain.MQTT, Type: domain.TypeFan, Addr: "esp32/living/fan"}
	d, cancel := startDriver(t, b, dev)
	defer cancel()

	payload, _ := json.Marshal(statePayload{On: boolPtr(true), Level: intPtr(40)})
	if err := d.server.Publish("esp32/living/fan/state", payload, false, 0); err != nil {
		t.Fatal(err)
	}

	select {
	case e := <-events:
		if e.DeviceID != "fan1" || e.Kind != domain.EventStateChanged {
			t.Fatalf("event = %+v", e)
		}
		if e.State.On == nil || !*e.State.On || e.State.Level == nil || *e.State.Level != 40 {
			t.Fatalf("state = %+v", e.State)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event from state publication on the addr-based topic")
	}
}

func TestApplyPublishesSetTopic(t *testing.T) {
	b := bus.New(8)
	dev := domain.Device{ID: "fan1", Integration: domain.MQTT, Type: domain.TypeFan, Addr: "esp32/living/fan"}
	d, cancel := startDriver(t, b, dev)
	defer cancel()

	got := make(chan packets.Packet, 1)
	if err := d.server.Subscribe("esp32/living/fan/set", 99, func(_ *mochi.Client, _ packets.Subscription, pk packets.Packet) {
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
		t.Fatal("no publication on the addr-based set topic")
	}

	// Unknown action publishes nothing and does not error.
	if err := d.Apply(domain.Command{DeviceID: "fan1", Action: "bogus"}); err != nil {
		t.Fatal(err)
	}
	// Unknown device errors rather than publishing to a bogus topic.
	if err := d.Apply(domain.SetOn("ghost", true)); err == nil {
		t.Fatal("expected error for unknown device")
	}
}

func boolPtr(b bool) *bool { return &b }
func intPtr(i int) *int    { return &i }
