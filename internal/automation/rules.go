package automation

import "github.com/sh5080/home-hub/internal/domain"

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
