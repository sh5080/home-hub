package domain

// EventKind classifies a bus event.
type EventKind string

const (
	EventStateChanged  EventKind = "state_changed"
	EventDeviceOnline  EventKind = "device_online"
	EventDeviceOffline EventKind = "device_offline"
	EventButton        EventKind = "button" // a stateless button/action press
)

// Press values for EventButton events.
const (
	PressSingle = "single"
	PressDouble = "double"
	PressHold   = "hold"
)

// Event is emitted by adapters when a device changes.
type Event struct {
	DeviceID string
	Kind     EventKind
	State    State
	Press    string // set when Kind == EventButton: PressSingle/Double/Hold
}
