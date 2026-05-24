package matter

import (
	"testing"

	"github.com/sh5080/home-hub/internal/domain"
)

// fakeDriver is a Driver whose LiftPercent is scripted.
type fakeDriver struct {
	pct   int
	err   error
	calls int
}

func (f *fakeDriver) Open() error              { return nil }
func (f *fakeDriver) Close() error             { return nil }
func (f *fakeDriver) SetLiftPercent(int) error { return nil }
func (f *fakeDriver) LiftPercent() (int, error) {
	f.calls++
	return f.pct, f.err
}

func collect(reg *Registry) (*Poller, *[]domain.Event) {
	var events []domain.Event
	p := NewPoller(reg, func(e domain.Event) { events = append(events, e) }, 0, nil)
	return p, &events
}

func TestPollerPublishesChanges(t *testing.T) {
	reg := NewRegistry()
	fd := &fakeDriver{pct: 42}
	reg.Set("blind1", fd)
	p, events := collect(reg)

	p.pollOnce()
	if len(*events) != 1 {
		t.Fatalf("events = %d, want 1", len(*events))
	}
	e := (*events)[0]
	if e.DeviceID != "blind1" || e.Kind != domain.EventStateChanged || e.State.Position == nil || *e.State.Position != 42 {
		t.Fatalf("event = %+v", e)
	}

	// Unchanged position must not re-publish.
	p.pollOnce()
	if len(*events) != 1 {
		t.Fatalf("unchanged poll re-published: %d events", len(*events))
	}

	// A new position publishes again.
	fd.pct = 80
	p.pollOnce()
	if len(*events) != 2 || *(*events)[1].State.Position != 80 {
		t.Fatalf("events after change = %+v", *events)
	}
}

func TestPollerSkipsDelegated(t *testing.T) {
	reg := NewRegistry()
	reg.Set("d1", NewDelegated(func() {}, func() {}, nil))
	p, events := collect(reg)

	p.pollOnce()
	if len(*events) != 0 {
		t.Fatalf("delegated device should not publish: %+v", *events)
	}
}
