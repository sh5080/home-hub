package rf

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/sh5080/home-hub/internal/bus"
	"github.com/sh5080/home-hub/internal/domain"
)

type fakePub struct {
	mu   sync.Mutex
	sent []sentMsg
	ch   chan sentMsg
}

type sentMsg struct {
	topic   string
	payload string
}

func newFakePub() *fakePub { return &fakePub{ch: make(chan sentMsg, 16)} }

func (f *fakePub) Publish(topic string, payload []byte, retain bool) error {
	m := sentMsg{topic, string(payload)}
	f.mu.Lock()
	f.sent = append(f.sent, m)
	f.mu.Unlock()
	f.ch <- m
	return nil
}

func testFan() Device {
	return Device{
		ID: "living_fan", Type: domain.TypeFan, Base: "rf447",
		Buttons: FanButtons{Off: 7, Speeds: map[int]int{33: 4, 66: 2, 100: 9}},
	}
}

func testBlind() Device {
	return Device{ID: "bedroom_blind", Type: domain.TypeCover, Base: "rf447", Channel: 1}
}

func TestCoverMapping(t *testing.T) {
	cases := []struct {
		cmd     domain.Command
		payload string
		pos     int
	}{
		{domain.SetPosition("bedroom_blind", 100), "OPEN", 100},
		{domain.SetPosition("bedroom_blind", 0), "CLOSE", 0},
		{domain.SetPosition("bedroom_blind", 47), "STOP", 47},
		{domain.SetOn("bedroom_blind", true), "OPEN", 100},
		{domain.SetOn("bedroom_blind", false), "CLOSE", 0},
	}
	for _, c := range cases {
		tx := coverTx(testBlind(), c.cmd)
		if tx.topic != "rf447/blind/1/set" {
			t.Errorf("%v: topic = %q", c.cmd, tx.topic)
		}
		if tx.payload != c.payload {
			t.Errorf("%v: payload = %q, want %q", c.cmd, tx.payload, c.payload)
		}
		if tx.event.State.Position == nil || *tx.event.State.Position != c.pos {
			t.Errorf("%v: optimistic position = %v, want %d", c.cmd, tx.event.State.Position, c.pos)
		}
	}
}

func TestFanMapping(t *testing.T) {
	cases := []struct {
		cmd   domain.Command
		topic string
		on    bool
		level int
	}{
		{domain.SetOn("living_fan", false), "rf447/fan/7/press", false, 0},
		{domain.SetLevel("living_fan", 0), "rf447/fan/7/press", false, 0},
		{domain.SetOn("living_fan", true), "rf447/fan/4/press", true, 33},  // lowest speed
		{domain.SetLevel("living_fan", 40), "rf447/fan/4/press", true, 33}, // nearest
		{domain.SetLevel("living_fan", 50), "rf447/fan/2/press", true, 66}, // 66 is closer
		{domain.SetLevel("living_fan", 100), "rf447/fan/9/press", true, 100},
	}
	for _, c := range cases {
		tx := fanTx(testFan(), c.cmd)
		if tx.topic != c.topic {
			t.Errorf("%v: topic = %q, want %q", c.cmd, tx.topic, c.topic)
		}
		s := tx.event.State
		if s.On == nil || *s.On != c.on || s.Level == nil || *s.Level != c.level {
			t.Errorf("%v: optimistic state = %+v, want on=%v level=%d", c.cmd, s, c.on, c.level)
		}
	}
}

func TestNearestSpeedTieGoesUp(t *testing.T) {
	level, btn := nearestSpeed(map[int]int{40: 1, 60: 2}, 50)
	if level != 60 || btn != 2 {
		t.Errorf("tie at 50 = level %d btn %d, want 60/2", level, btn)
	}
}

// The queue keeps only the newest pending command per device, so a HomeKit
// slider burst collapses to one transmission.
func TestQueueCoalesces(t *testing.T) {
	d := New([]Device{testBlind(), testFan()}, newFakePub(), bus.New(4), slog.Default())
	if err := d.Apply(domain.SetPosition("bedroom_blind", 100)); err != nil {
		t.Fatal(err)
	}
	if err := d.Apply(domain.SetLevel("living_fan", 100)); err != nil {
		t.Fatal(err)
	}
	if err := d.Apply(domain.SetPosition("bedroom_blind", 0)); err != nil {
		t.Fatal(err)
	}

	first, ok := d.pop()
	if !ok || first.payload != "CLOSE" {
		t.Fatalf("first tx = %+v, want the blind's latest command (CLOSE)", first)
	}
	second, ok := d.pop()
	if !ok || second.topic != "rf447/fan/9/press" {
		t.Fatalf("second tx = %+v, want the fan press", second)
	}
	if _, ok := d.pop(); ok {
		t.Fatal("queue should be empty")
	}
}

func TestStartTransmitsAndPublishesOptimisticState(t *testing.T) {
	pub := newFakePub()
	b := bus.New(4)
	events := b.SubscribeEvents()
	d := New([]Device{testBlind()}, pub, b, slog.Default())
	d.pace = func(context.Context, time.Duration) {} // no cooldown in tests

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { _ = d.Start(ctx); close(done) }()

	if err := d.Apply(domain.SetPosition("bedroom_blind", 100)); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-pub.ch:
		if m.topic != "rf447/blind/1/set" || m.payload != "OPEN" {
			t.Fatalf("sent %+v", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("nothing transmitted")
	}
	select {
	case e := <-events:
		if e.DeviceID != "bedroom_blind" || e.State.Position == nil || *e.State.Position != 100 {
			t.Fatalf("event %+v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no optimistic state event")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not stop on cancel")
	}
}

func TestApplyUnknownDevice(t *testing.T) {
	d := New(nil, newFakePub(), bus.New(4), slog.Default())
	if err := d.Apply(domain.SetOn("ghost", true)); err == nil {
		t.Fatal("want error for unknown device")
	}
}
