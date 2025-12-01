package matter

import "sync"

// Registry maps device IDs to their Matter driver (delegated today, native
// go-matter later). It lets the rest of the hub target Matter devices without
// knowing which driver backs them.
type Registry struct {
	mu      sync.RWMutex
	drivers map[string]Driver
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{drivers: make(map[string]Driver)}
}

// Set registers a driver for a device ID.
func (r *Registry) Set(id string, d Driver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.drivers[id] = d
}

// Get returns the driver for a device ID.
func (r *Registry) Get(id string) (Driver, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.drivers[id]
	return d, ok
}

// IDs returns all registered device IDs.
func (r *Registry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.drivers))
	for id := range r.drivers {
		out = append(out, id)
	}
	return out
}
