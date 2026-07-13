package homekit

import (
	"github.com/brutella/hap/accessory"
	"github.com/brutella/hap/characteristic"
	"github.com/brutella/hap/service"

	"github.com/sh5080/home-hub/internal/domain"
)

// devAccessory bundles a HAP accessory with a closure that pushes domain state
// onto its characteristics.
type devAccessory struct {
	a     *accessory.A
	apply func(domain.State)
}

// buildAccessory creates a HAP accessory for d, wiring writable characteristics
// to publish commands through onCmd.
func (br *Bridge) buildAccessory(d domain.Device, onCmd func(domain.Command)) devAccessory {
	info := accessory.Info{
		Name:         d.Name,
		SerialNumber: d.ID,
		Manufacturer: "home-hub",
		Model:        string(d.Type),
	}
	switch d.Type {
	case domain.TypeLight:
		a := accessory.NewLightbulb(info)
		a.Lightbulb.On.OnValueRemoteUpdate(func(on bool) { onCmd(domain.SetOn(d.ID, on)) })
		apply := func(s domain.State) {
			if s.On != nil {
				a.Lightbulb.On.SetValue(*s.On)
			}
		}
		// Optional light capabilities (opt-in via Features) so a plain downlight
		// stays on/off while the BLE ceiling-fan light exposes dimming/CCT/RGB.
		if d.Has("brightness") { // brightness rides the domain Level (0..100)
			br := characteristic.NewBrightness()
			a.Lightbulb.AddC(br.C)
			br.OnValueRemoteUpdate(func(v int) { onCmd(domain.SetLevel(d.ID, v)) })
			prev := apply
			apply = func(s domain.State) {
				prev(s)
				if s.Level != nil {
					br.SetValue(*s.Level)
				}
			}
		}
		if d.Has("colortemp") {
			ct := characteristic.NewColorTemperature()
			a.Lightbulb.AddC(ct.C)
			ct.OnValueRemoteUpdate(func(v int) { onCmd(domain.SetColorTemp(d.ID, v)) })
			prev := apply
			apply = func(s domain.State) {
				prev(s)
				if s.ColorTemp != nil {
					ct.SetValue(*s.ColorTemp)
				}
			}
		}
		if d.Has("color") {
			hue := characteristic.NewHue()
			sat := characteristic.NewSaturation()
			a.Lightbulb.AddC(hue.C)
			a.Lightbulb.AddC(sat.C)
			// HomeKit writes hue and saturation separately; emit one color
			// command using both current values so the ESP32 gets a full color.
			emit := func() { onCmd(domain.SetColor(d.ID, hue.Value(), sat.Value())) }
			hue.OnValueRemoteUpdate(func(float64) { emit() })
			sat.OnValueRemoteUpdate(func(float64) { emit() })
			prev := apply
			apply = func(s domain.State) {
				prev(s)
				if s.Hue != nil {
					hue.SetValue(*s.Hue)
				}
				if s.Sat != nil {
					sat.SetValue(*s.Sat)
				}
			}
		}
		return devAccessory{a: a.A, apply: apply}
	case domain.TypeFan:
		a := accessory.NewFan(info)
		a.Fan.On.OnValueRemoteUpdate(func(on bool) { onCmd(domain.SetOn(d.ID, on)) })
		// RotationSpeed (0..100) maps onto the domain level, so multi-speed
		// fans (e.g. RF ceiling fans behind an ESP32 bridge) are adjustable
		// from the Home app, not just on/off.
		speed := characteristic.NewRotationSpeed()
		a.Fan.AddC(speed.C)
		speed.OnValueRemoteUpdate(func(v float64) { onCmd(domain.SetLevel(d.ID, int(v))) })
		apply := func(s domain.State) {
			if s.On != nil {
				a.Fan.On.SetValue(*s.On)
			}
			if s.Level != nil {
				speed.SetValue(float64(*s.Level))
			}
		}
		if d.Has("direction") { // reversible ceiling fan (summer/winter spin)
			dir := characteristic.NewRotationDirection()
			a.Fan.AddC(dir.C)
			dir.OnValueRemoteUpdate(func(v int) { onCmd(domain.SetDirection(d.ID, v)) })
			prev := apply
			apply = func(s domain.State) {
				prev(s)
				if s.Direction != nil {
					dir.SetValue(*s.Direction)
				}
			}
		}
		return devAccessory{a: a.A, apply: apply}
	case domain.TypeCover:
		a := accessory.NewWindowCovering(info)
		a.WindowCovering.TargetPosition.OnValueRemoteUpdate(func(p int) { onCmd(domain.SetPosition(d.ID, p)) })
		return devAccessory{a: a.A, apply: func(s domain.State) {
			if s.Position != nil {
				// Reflect both current and target so the Home app settles
				// instead of showing a perpetual "opening…" state.
				a.WindowCovering.CurrentPosition.SetValue(*s.Position)
				a.WindowCovering.TargetPosition.SetValue(*s.Position)
			}
		}}
	case domain.TypeSensor:
		a := accessory.NewTemperatureSensor(info)
		return devAccessory{a: a.A, apply: func(s domain.State) {
			if s.Value != nil {
				a.TempSensor.CurrentTemperature.SetValue(*s.Value)
			}
		}}
	case domain.TypeHumidity:
		// A relative-humidity sensor: State.Value carries percent RH so the Home
		// app shows humidity, not a bogus temperature.
		a := accessory.New(info, accessory.TypeSensor)
		hs := service.NewHumiditySensor()
		a.AddS(hs.S)
		return devAccessory{a: a, apply: func(s domain.State) {
			if s.Value != nil {
				hs.CurrentRelativeHumidity.SetValue(*s.Value)
			}
		}}
	default: // TypeSwitch
		a := accessory.NewSwitch(info)
		a.Switch.On.OnValueRemoteUpdate(func(on bool) { onCmd(domain.SetOn(d.ID, on)) })
		return devAccessory{a: a.A, apply: func(s domain.State) {
			if s.On != nil {
				a.Switch.On.SetValue(*s.On)
			}
		}}
	}
}
