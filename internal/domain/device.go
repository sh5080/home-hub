// Package domain holds protocol-agnostic core types. It has no external deps.
package domain

// DeviceType is the high-level kind of a device.
type DeviceType string

const (
	TypeSwitch   DeviceType = "switch"
	TypeLight    DeviceType = "light"
	TypeFan      DeviceType = "fan"
	TypeCover    DeviceType = "cover"
	TypeSensor   DeviceType = "sensor"   // temperature sensor
	TypeHumidity DeviceType = "humidity" // relative-humidity sensor
)

// Integration identifies which adapter owns a device.
type Integration string

const (
	Zigbee Integration = "zigbee"
	MQTT   Integration = "mqtt"
	Matter Integration = "matter"
	// RF devices sit behind an ESP32+CC1101 bridge that replays/generates the
	// original remote's 447 MHz frames. Transmit-only: no state ever comes back.
	RF Integration = "rf"
)

// Device is a logical device exposed by the hub, independent of protocol.
type Device struct {
	ID          string      `yaml:"id"`
	Name        string      `yaml:"name"`
	Integration Integration `yaml:"integration"`
	Type        DeviceType  `yaml:"type"`
	Addr        string      `yaml:"addr"` // zigbee IEEE addr | mqtt topic | matter node id
	// Endpoint distinguishes sub-units behind one address, e.g. the gangs of a
	// multi-gang Zigbee wall switch (H2 2-gang: endpoints 1 and 2). 0 means the
	// integration default (Zigbee: endpoint 1).
	Endpoint uint8 `yaml:"endpoint,omitempty"`
	// Decoupled requests decoupled mode on an Aqara Zigbee switch: the relay is
	// detached from the paddle so pressing it emits a button event instead of
	// toggling the load, letting a rule drive any device. Applied on join.
	Decoupled bool `yaml:"decoupled,omitempty"`
	// Features opts a device into optional HomeKit capabilities beyond the type
	// default. A plain light is on/off; add "brightness", "colortemp", "color"
	// for a dimmable/tunable/RGB light (e.g. the BLE ceiling-fan light). A fan
	// adds "direction" for reversible spin. Unlisted features are not exposed,
	// so simple downlights stay on/off.
	Features []string `yaml:"features,omitempty"`
}

// Has reports whether the device opted into the named feature.
func (d Device) Has(feature string) bool {
	for _, f := range d.Features {
		if f == feature {
			return true
		}
	}
	return false
}
