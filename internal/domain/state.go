package domain

// State is the current state of a device. Fields are optional and populated
// according to the device Type (a switch uses On, a cover uses Position, etc.).
type State struct {
	On       *bool    `json:"on,omitempty"`
	Position *int     `json:"position,omitempty"` // cover: 0..100 (percent open)
	Value    *float64 `json:"value,omitempty"`    // sensor reading
}

// BoolPtr / IntPtr / FloatPtr are small helpers for building State values.
func BoolPtr(b bool) *bool      { return &b }
func IntPtr(i int) *int         { return &i }
func FloatPtr(f float64) *float64 { return &f }
