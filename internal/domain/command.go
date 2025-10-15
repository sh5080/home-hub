package domain

// Action is a command verb targeting a device.
type Action string

const (
	ActionSetOn       Action = "set_on"
	ActionSetPosition Action = "set_position"
)

// Command instructs an adapter to change a device.
type Command struct {
	DeviceID string
	Action   Action
	Value    any
}

// SetOn builds an on/off command.
func SetOn(id string, on bool) Command {
	return Command{DeviceID: id, Action: ActionSetOn, Value: on}
}

// SetPosition builds a cover-position command (0..100).
func SetPosition(id string, pct int) Command {
	return Command{DeviceID: id, Action: ActionSetPosition, Value: pct}
}
