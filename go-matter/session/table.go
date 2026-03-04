package session

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"sync"
)

// Table holds active sessions keyed by local session id and allocates fresh
// ids. It is safe for concurrent use.
type Table struct {
	mu       sync.RWMutex
	sessions map[uint16]*Secure
}

// NewTable returns an empty session table.
func NewTable() *Table {
	return &Table{sessions: make(map[uint16]*Secure)}
}

// Add stores s under its LocalSessionID.
func (t *Table) Add(s *Secure) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sessions[s.LocalSessionID] = s
}

// Get returns the session addressed by localID.
func (t *Table) Get(localID uint16) (*Secure, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	s, ok := t.sessions[localID]
	return s, ok
}

// Remove deletes a session.
func (t *Table) Remove(localID uint16) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.sessions, localID)
}

// Len returns the number of active sessions.
func (t *Table) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.sessions)
}

// AllocID returns a random unused session id in [1, 65535]. Session id 0 is
// reserved for the unsecured session (Spec 4.4.1.2).
func (t *Table) AllocID() (uint16, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for tries := 0; tries < 1000; tries++ {
		var b [2]byte
		if _, err := rand.Read(b[:]); err != nil {
			return 0, err
		}
		id := binary.BigEndian.Uint16(b[:])
		if id == 0 {
			continue
		}
		if _, exists := t.sessions[id]; !exists {
			return id, nil
		}
	}
	return 0, errors.New("session: no free session id available")
}
