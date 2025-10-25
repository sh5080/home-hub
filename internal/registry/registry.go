// Package registry keeps the set of known devices and their latest state.
package registry

import (
	"sync"

	"github.com/sh5080/home-hub/internal/domain"
)

type entry struct {
	device domain.Device
	state  domain.State
}

// Registry is a concurrency-safe store of devices and their current state.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]*entry
}

// New creates an empty Registry.
func New() *Registry {
	return &Registry{entries: make(map[string]*entry)}
}

// Add registers a device (idempotent by ID).
func (r *Registry) Add(d domain.Device) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.entries[d.ID]; !ok {
		r.entries[d.ID] = &entry{device: d}
	}
}

// Get returns a device by ID.
func (r *Registry) Get(id string) (domain.Device, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[id]
	if !ok {
		return domain.Device{}, false
	}
	return e.device, true
}

// List returns all registered devices.
func (r *Registry) List() []domain.Device {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Device, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e.device)
	}
	return out
}

// SetState updates the cached state for a device.
func (r *Registry) SetState(id string, s domain.State) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.entries[id]; ok {
		e.state = s
	}
}

// State returns the cached state for a device.
func (r *Registry) State(id string) (domain.State, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[id]
	if !ok {
		return domain.State{}, false
	}
	return e.state, true
}
