// Package driver defines the port that every protocol adapter implements.
package driver

import (
	"context"

	"github.com/sh5080/home-hub/internal/domain"
)

// Driver is a protocol adapter. Adapters translate domain commands into
// protocol actions and publish device state back onto the event bus.
type Driver interface {
	// Name identifies the adapter (e.g. "zigbee", "mqtt", "homekit").
	Name() string
	// Start brings the adapter up and blocks until ctx is cancelled.
	Start(ctx context.Context) error
	// Apply executes a command targeting a device this adapter owns.
	Apply(cmd domain.Command) error
}
