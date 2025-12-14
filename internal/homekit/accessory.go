package homekit

import (
	"github.com/brutella/hap/accessory"

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
		return devAccessory{a: a.A, apply: func(s domain.State) {
			if s.On != nil {
				a.Lightbulb.On.SetValue(*s.On)
			}
		}}
	case domain.TypeFan:
		a := accessory.NewFan(info)
		a.Fan.On.OnValueRemoteUpdate(func(on bool) { onCmd(domain.SetOn(d.ID, on)) })
		return devAccessory{a: a.A, apply: func(s domain.State) {
			if s.On != nil {
				a.Fan.On.SetValue(*s.On)
			}
		}}
	case domain.TypeCover:
		a := accessory.NewWindowCovering(info)
		a.WindowCovering.TargetPosition.OnValueRemoteUpdate(func(p int) { onCmd(domain.SetPosition(d.ID, p)) })
		return devAccessory{a: a.A, apply: func(s domain.State) {
			if s.Position != nil {
				a.WindowCovering.CurrentPosition.SetValue(*s.Position)
			}
		}}
	case domain.TypeSensor:
		a := accessory.NewTemperatureSensor(info)
		return devAccessory{a: a.A, apply: func(s domain.State) {
			if s.Value != nil {
				a.TempSensor.CurrentTemperature.SetValue(*s.Value)
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
