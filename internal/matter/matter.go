// Package matter abstracts Matter control behind a Driver interface.
//
// Two implementations are planned:
//
//   - DelegatedDriver: HomeKit owns the Matter device; the hub can only fire
//     trigger automations (no arbitrary position, no state read). Used today.
//   - GoMatterDriver:  a native controller (separate go-matter module) that
//     owns the device directly. Swapped in when go-matter reaches operational
//     control (see docs/ARCHITECTURE.md §9).
package matter

import (
	"errors"
	"log/slog"
)

// ErrUnsupported is returned by drivers that cannot perform an operation.
var ErrUnsupported = errors.New("matter: operation not supported by this driver")

// Driver controls a single Matter device (e.g. a window covering).
type Driver interface {
	Open() error
	Close() error
	SetLiftPercent(p int) error // 0..100
	LiftPercent() (int, error)
}

// DelegatedDriver drives a HomeKit-owned Matter device via virtual triggers.
// pressOpen/pressClose pulse HAP virtual switches that HomeKit automations
// react to (typically obtained from homekit.Bridge.RegisterTrigger).
type DelegatedDriver struct {
	pressOpen  func()
	pressClose func()
	log        *slog.Logger
}

// NewDelegated builds a delegated driver from trigger-press callbacks.
func NewDelegated(pressOpen, pressClose func(), log *slog.Logger) *DelegatedDriver {
	return &DelegatedDriver{pressOpen: pressOpen, pressClose: pressClose, log: log}
}

// Open fires the "open" trigger automation.
func (d *DelegatedDriver) Open() error { d.pressOpen(); return nil }

// Close fires the "close" trigger automation.
func (d *DelegatedDriver) Close() error { d.pressClose(); return nil }

// SetLiftPercent is not available under HomeKit delegation.
func (d *DelegatedDriver) SetLiftPercent(int) error { return ErrUnsupported }

// LiftPercent is not available under HomeKit delegation (no state read-back).
func (d *DelegatedDriver) LiftPercent() (int, error) { return 0, ErrUnsupported }
