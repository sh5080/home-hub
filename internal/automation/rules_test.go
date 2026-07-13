package automation

import (
	"testing"

	"github.com/sh5080/home-hub/internal/domain"
)

func TestButtonRuleToggle(t *testing.T) {
	state := domain.State{On: domain.BoolPtr(true)}
	getter := func(id string) (domain.State, bool) { return state, id == "fan" }
	r := ButtonRule("sw1", domain.PressSingle, "fan", "toggle", 0, getter)

	// Wrong press kind or device: no command.
	if got := r(domain.Event{DeviceID: "sw1", Kind: domain.EventButton, Press: domain.PressDouble}); got != nil {
		t.Fatalf("double press should not match: %+v", got)
	}
	if got := r(domain.Event{DeviceID: "other", Kind: domain.EventButton, Press: domain.PressSingle}); got != nil {
		t.Fatalf("other device should not match: %+v", got)
	}

	// fan currently on -> toggle off.
	cmds := r(domain.Event{DeviceID: "sw1", Kind: domain.EventButton, Press: domain.PressSingle})
	if len(cmds) != 1 || cmds[0].DeviceID != "fan" || cmds[0].Action != domain.ActionSetOn {
		t.Fatalf("cmds = %+v", cmds)
	}
	if on, _ := cmds[0].Value.(bool); on {
		t.Fatal("toggle from on must produce off")
	}

	// fan off -> toggle on.
	state = domain.State{On: domain.BoolPtr(false)}
	cmds = r(domain.Event{DeviceID: "sw1", Kind: domain.EventButton, Press: domain.PressSingle})
	if on, _ := cmds[0].Value.(bool); !on {
		t.Fatal("toggle from off must produce on")
	}
}

func TestButtonRuleCoverActions(t *testing.T) {
	getter := func(string) (domain.State, bool) { return domain.State{}, false }

	open := ButtonRule("sw1", domain.PressSingle, "blind", "open", 0, getter)
	cmds := open(domain.Event{DeviceID: "sw1", Kind: domain.EventButton, Press: domain.PressSingle})
	if len(cmds) != 1 || cmds[0].Action != domain.ActionSetPosition || cmds[0].Value.(int) != 100 {
		t.Fatalf("open cmds = %+v", cmds)
	}

	pos := ButtonRule("sw1", domain.PressDouble, "blind", "position", 40, getter)
	cmds = pos(domain.Event{DeviceID: "sw1", Kind: domain.EventButton, Press: domain.PressDouble})
	if len(cmds) != 1 || cmds[0].Value.(int) != 40 {
		t.Fatalf("position cmds = %+v", cmds)
	}
}

func TestCycleRuleSteps(t *testing.T) {
	// off → 3단(50) → 6단(100) → off, wrapping. Starts at states[0]=off.
	r := CycleRule("btn", domain.PressSingle, "fan", []int{0, 50, 100}, "")
	press := domain.Event{DeviceID: "btn", Kind: domain.EventButton, Press: domain.PressSingle}

	step := func(want int, wantOff bool) {
		t.Helper()
		cmds := r(press)
		if len(cmds) != 1 || cmds[0].DeviceID != "fan" {
			t.Fatalf("cmds = %+v", cmds)
		}
		if wantOff {
			if cmds[0].Action != domain.ActionSetOn || cmds[0].Value.(bool) {
				t.Fatalf("want off, got %+v", cmds[0])
			}
			return
		}
		if cmds[0].Action != domain.ActionSetLevel || cmds[0].Value.(int) != want {
			t.Fatalf("want level %d, got %+v", want, cmds[0])
		}
	}
	step(50, false)  // 1st press: off → 3단
	step(100, false) // 2nd: → 6단
	step(0, true)    // 3rd: → off
	step(50, false)  // wraps: → 3단
}

func TestCycleRuleIgnoresAndResets(t *testing.T) {
	r := CycleRule("btn", domain.PressSingle, "light", []int{0, 100}, "power")
	press := domain.Event{DeviceID: "btn", Kind: domain.EventButton, Press: domain.PressSingle}

	// Wrong device / press kind are ignored.
	if got := r(domain.Event{DeviceID: "other", Kind: domain.EventButton, Press: domain.PressSingle}); got != nil {
		t.Fatalf("other device should not match: %+v", got)
	}
	if got := r(domain.Event{DeviceID: "btn", Kind: domain.EventButton, Press: domain.PressDouble}); got != nil {
		t.Fatalf("wrong press should not match: %+v", got)
	}

	// First press turns on; second returns to off.
	if cmds := r(press); cmds[0].Action != domain.ActionSetLevel || cmds[0].Value.(int) != 100 {
		t.Fatalf("first press = %+v", cmds)
	}
	// Power off resets the sequence (no command emitted).
	off := domain.Event{DeviceID: "power", Kind: domain.EventStateChanged, State: domain.State{On: domain.BoolPtr(false)}}
	if got := r(off); got != nil {
		t.Fatalf("power-off should emit nothing: %+v", got)
	}
	// After reset, next press starts from states[0] again → states[1]=on.
	if cmds := r(press); cmds[0].Action != domain.ActionSetLevel || cmds[0].Value.(int) != 100 {
		t.Fatalf("post-reset press should turn on again: %+v", cmds)
	}
}

func TestThresholdRuleHysteresis(t *testing.T) {
	r := ThresholdRule("humid", "dehumidifier", 65, 55)
	ev := func(v float64) domain.Event {
		return domain.Event{DeviceID: "humid", Kind: domain.EventStateChanged, State: domain.State{Value: domain.FloatPtr(v)}}
	}

	// Rising through the band: nothing until `above` is reached.
	if got := r(ev(60)); got != nil {
		t.Fatalf("mid-band should not trigger: %+v", got)
	}
	cmds := r(ev(66))
	if len(cmds) != 1 || cmds[0].Value.(bool) != true {
		t.Fatalf("above should turn on: %+v", cmds)
	}
	// Staying high or dipping into the band: no repeat commands.
	if got := r(ev(70)); got != nil {
		t.Fatalf("already on, no repeat: %+v", got)
	}
	if got := r(ev(60)); got != nil {
		t.Fatalf("hysteresis band must not turn off: %+v", got)
	}
	// Falling to `below`: off exactly once.
	cmds = r(ev(54))
	if len(cmds) != 1 || cmds[0].Value.(bool) != false {
		t.Fatalf("below should turn off: %+v", cmds)
	}
	if got := r(ev(50)); got != nil {
		t.Fatalf("already off, no repeat: %+v", got)
	}
}
