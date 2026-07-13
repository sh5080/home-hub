package domain

// State is the current state of a device. Fields are optional and populated
// according to the device Type (a switch uses On, a cover uses Position, etc.).
type State struct {
	On        *bool    `json:"on,omitempty"`
	Position  *int     `json:"position,omitempty"`  // cover: 0..100 (percent open)
	Level     *int     `json:"level,omitempty"`     // light/fan: 0..100 (brightness/speed)
	Value     *float64 `json:"value,omitempty"`     // sensor reading
	ColorTemp *int     `json:"colortemp,omitempty"` // light: white temperature, mireds
	Hue       *float64 `json:"hue,omitempty"`       // light: RGB hue 0..360
	Sat       *float64 `json:"sat,omitempty"`       // light: RGB saturation 0..100
	Direction *int     `json:"direction,omitempty"` // fan: 0=clockwise, 1=counter
}

// BoolPtr / IntPtr / FloatPtr are small helpers for building State values.
func BoolPtr(b bool) *bool        { return &b }
func IntPtr(i int) *int           { return &i }
func FloatPtr(f float64) *float64 { return &f }
