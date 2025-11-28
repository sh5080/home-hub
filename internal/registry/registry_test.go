package registry

import (
	"testing"

	"github.com/sh5080/home-hub/internal/domain"
)

func TestAddGetList(t *testing.T) {
	r := New()
	r.Add(domain.Device{ID: "a", Integration: domain.Zigbee, Type: domain.TypeSwitch})
	r.Add(domain.Device{ID: "a"}) // duplicate id ignored

	if got := len(r.List()); got != 1 {
		t.Fatalf("List len = %d, want 1", got)
	}
	d, ok := r.Get("a")
	if !ok || d.Integration != domain.Zigbee {
		t.Fatalf("Get(a) = %+v, ok=%v", d, ok)
	}
	if _, ok := r.Get("missing"); ok {
		t.Fatal("Get(missing) should be false")
	}
}

func TestSetState(t *testing.T) {
	r := New()
	r.Add(domain.Device{ID: "a"})
	r.SetState("a", domain.State{On: domain.BoolPtr(true)})

	s, ok := r.State("a")
	if !ok || s.On == nil || *s.On != true {
		t.Fatalf("State(a) = %+v, ok=%v", s, ok)
	}
	// state on an unknown device is not found
	if _, ok := r.State("missing"); ok {
		t.Fatal("State(missing) should be false")
	}
}
