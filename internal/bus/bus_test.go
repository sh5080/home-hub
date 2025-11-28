package bus

import (
	"testing"

	"github.com/sh5080/home-hub/internal/domain"
)

func TestPublishCommandFanOut(t *testing.T) {
	b := New(4)
	s1 := b.SubscribeCommands()
	s2 := b.SubscribeCommands()

	b.PublishCommand(domain.SetOn("dev1", true))

	for i, ch := range []<-chan domain.Command{s1, s2} {
		select {
		case c := <-ch:
			if c.DeviceID != "dev1" || c.Action != domain.ActionSetOn {
				t.Fatalf("subscriber %d got unexpected command: %+v", i, c)
			}
		default:
			t.Fatalf("subscriber %d received nothing", i)
		}
	}
}

func TestPublishEventFanOut(t *testing.T) {
	b := New(2)
	s := b.SubscribeEvents()
	b.PublishEvent(domain.Event{DeviceID: "d", Kind: domain.EventStateChanged})
	select {
	case e := <-s:
		if e.DeviceID != "d" {
			t.Fatalf("unexpected event: %+v", e)
		}
	default:
		t.Fatal("no event received")
	}
}

func TestPublishNonBlockingWhenFull(t *testing.T) {
	b := New(1)
	_ = b.SubscribeCommands() // never drained
	// Publishing more than the buffer must not block or panic.
	for i := 0; i < 5; i++ {
		b.PublishCommand(domain.SetOn("d", true))
	}
}
