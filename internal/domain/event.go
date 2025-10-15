package domain

// EventKind classifies a bus event.
type EventKind string

const (
	EventStateChanged  EventKind = "state_changed"
	EventDeviceOnline  EventKind = "device_online"
	EventDeviceOffline EventKind = "device_offline"
)

// Event is emitted by adapters when a device changes.
type Event struct {
	DeviceID string
	Kind     EventKind
	State    State
}
