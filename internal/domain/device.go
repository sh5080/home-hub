// Package domain holds protocol-agnostic core types. It has no external deps.
package domain

// DeviceType is the high-level kind of a device.
type DeviceType string

const (
	TypeSwitch DeviceType = "switch"
	TypeLight  DeviceType = "light"
	TypeFan    DeviceType = "fan"
	TypeCover  DeviceType = "cover"
	TypeSensor DeviceType = "sensor"
)

// Integration identifies which adapter owns a device.
type Integration string

const (
	Zigbee Integration = "zigbee"
	MQTT   Integration = "mqtt"
	Matter Integration = "matter"
)

// Device is a logical device exposed by the hub, independent of protocol.
type Device struct {
	ID          string      `yaml:"id"`
	Name        string      `yaml:"name"`
	Integration Integration `yaml:"integration"`
	Type        DeviceType  `yaml:"type"`
	Addr        string      `yaml:"addr"` // zigbee IEEE addr | mqtt topic | matter node id
}
