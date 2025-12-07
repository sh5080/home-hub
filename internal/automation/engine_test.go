package automation

import (
	"io"
	"log/slog"
	"testing"

	"github.com/sh5080/home-hub/internal/bus"
	"github.com/sh5080/home-hub/internal/domain"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestMirrorRule(t *testing.T) {
	r := MirrorRule("src", "dst")

	// unrelated device -> no commands
	if got := r(domain.Event{DeviceID: "other", Kind: domain.EventStateChanged, State: domain.State{On: domain.BoolPtr(true)}}); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}

	// matching state change -> mirror onto dst
	cmds := r(domain.Event{DeviceID: "src", Kind: domain.EventStateChanged, State: domain.State{On: domain.BoolPtr(true)}})
	if len(cmds) != 1 || cmds[0].DeviceID != "dst" || cmds[0].Action != domain.ActionSetOn {
		t.Fatalf("unexpected commands: %+v", cmds)
	}
	if on, _ := cmds[0].Value.(bool); !on {
		t.Fatal("expected mirrored value on=true")
	}
}

func TestEngineAdd(t *testing.T) {
	en := New(bus.New(1), discardLogger())
	en.Add(MirrorRule("a", "b"))
	if len(en.rules) != 1 {
		t.Fatalf("rules = %d, want 1", len(en.rules))
	}
}
