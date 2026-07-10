package automation

import "github.com/sh5080/home-hub/internal/domain"

// StateGetter looks up a device's last known state (registry.Registry.State).
type StateGetter func(id string) (domain.State, bool)

// MirrorRule returns a rule that mirrors the on/off state of srcID onto dstID.
// It is a small example of cross-device automation expressible entirely within
// the hub (both devices must be hub-owned, e.g. Zigbee/MQTT).
func MirrorRule(srcID, dstID string) Rule {
	return func(e domain.Event) []domain.Command {
		if e.DeviceID != srcID || e.Kind != domain.EventStateChanged || e.State.On == nil {
			return nil
		}
		return []domain.Command{domain.SetOn(dstID, *e.State.On)}
	}
}

// ButtonRule maps a button press on srcID (e.g. an Aqara switch in decoupled
// mode) to an action on dstID. This is how a wall button drives a device that
// is NOT wired through its relay: RF fan via MQTT, a Matter blind, another
// light. action: toggle | on | off | open | close | position (with value).
// toggle reads the destination's last known state via state.
func ButtonRule(srcID, press, dstID, action string, value int, state StateGetter) Rule {
	return func(e domain.Event) []domain.Command {
		if e.DeviceID != srcID || e.Kind != domain.EventButton || e.Press != press {
			return nil
		}
		switch action {
		case "toggle":
			on := false
			if s, ok := state(dstID); ok && s.On != nil {
				on = *s.On
			}
			return []domain.Command{domain.SetOn(dstID, !on)}
		case "on":
			return []domain.Command{domain.SetOn(dstID, true)}
		case "off":
			return []domain.Command{domain.SetOn(dstID, false)}
		case "open":
			return []domain.Command{domain.SetPosition(dstID, 100)}
		case "close":
			return []domain.Command{domain.SetPosition(dstID, 0)}
		case "position":
			return []domain.Command{domain.SetPosition(dstID, value)}
		}
		return nil
	}
}

// ThresholdRule turns dstID on when srcID's sensor value reaches above, and off
// when it falls to below (hysteresis so the device does not flap around one
// setpoint) — e.g. humidity ≥ 65% → dehumidifier on, ≤ 55% → off. Commands are
// only issued on crossings, tracked by the rule's own small state.
func ThresholdRule(srcID, dstID string, above, below float64) Rule {
	var on, known bool
	return func(e domain.Event) []domain.Command {
		if e.DeviceID != srcID || e.Kind != domain.EventStateChanged || e.State.Value == nil {
			return nil
		}
		v := *e.State.Value
		switch {
		case v >= above && (!known || !on):
			on, known = true, true
			return []domain.Command{domain.SetOn(dstID, true)}
		case v <= below && (!known || on):
			on, known = false, true
			return []domain.Command{domain.SetOn(dstID, false)}
		}
		return nil
	}
}
