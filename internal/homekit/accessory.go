package homekit

import "github.com/sh5080/home-hub/internal/domain"

// accessoryKind names the HAP accessory/service a device type maps to. It is
// the scaffold mapping used until real hap.Accessory types are wired in.
func accessoryKind(t domain.DeviceType) string {
	switch t {
	case domain.TypeSwitch:
		return "Switch"
	case domain.TypeLight:
		return "Lightbulb"
	case domain.TypeFan:
		return "Fan"
	case domain.TypeCover:
		return "WindowCovering"
	case domain.TypeSensor:
		return "TemperatureSensor"
	default:
		return "Switch"
	}
}
