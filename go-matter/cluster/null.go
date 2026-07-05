package cluster

import "errors"

// ErrNull is returned when a nullable attribute reports null (value unknown or
// not applicable, e.g. a covering's position while the motor is moving).
// Callers should treat it as "no sample", not as a failure.
var ErrNull = errors.New("cluster: attribute value is null")
