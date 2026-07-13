package domain

// Action is a command verb targeting a device.
type Action string

const (
	ActionSetOn        Action = "set_on"
	ActionSetPosition  Action = "set_position"
	ActionSetLevel     Action = "set_level"      // brightness / fan speed, 0..100
	ActionSetColorTemp Action = "set_colortemp"  // white color temperature, mireds
	ActionSetColor     Action = "set_color"      // RGB color, Value is HueSat
	ActionSetDirection Action = "set_direction"  // fan spin, 0=clockwise 1=counter
)

// HueSat is an RGB color as HomeKit expresses it: hue 0..360, sat 0..100.
type HueSat struct {
	Hue float64
	Sat float64
}

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

// SetLevel builds a brightness/fan-speed command (0..100).
func SetLevel(id string, level int) Command {
	return Command{DeviceID: id, Action: ActionSetLevel, Value: level}
}

// SetColorTemp builds a white color-temperature command (mireds).
func SetColorTemp(id string, mireds int) Command {
	return Command{DeviceID: id, Action: ActionSetColorTemp, Value: mireds}
}

// SetColor builds an RGB color command (hue 0..360, sat 0..100).
func SetColor(id string, hue, sat float64) Command {
	return Command{DeviceID: id, Action: ActionSetColor, Value: HueSat{Hue: hue, Sat: sat}}
}

// SetDirection builds a fan spin-direction command (0=clockwise, 1=counter).
func SetDirection(id string, dir int) Command {
	return Command{DeviceID: id, Action: ActionSetDirection, Value: dir}
}
