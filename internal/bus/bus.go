// Package bus is a tiny in-process publish/subscribe hub. Adapters communicate
// only through it and never reference each other directly.
package bus

import (
	"sync"

	"github.com/sh5080/home-hub/internal/domain"
)

// Bus fans out commands and events to all subscribers. Safe for concurrent use.
type Bus struct {
	mu      sync.RWMutex
	cmdSubs []chan domain.Command
	evtSubs []chan domain.Event
	buffer  int
}

// New creates a Bus. buffer is the per-subscriber channel buffer size.
func New(buffer int) *Bus {
	if buffer <= 0 {
		buffer = 16
	}
	return &Bus{buffer: buffer}
}

// SubscribeCommands returns a channel receiving every published command.
func (b *Bus) SubscribeCommands() <-chan domain.Command {
	ch := make(chan domain.Command, b.buffer)
	b.mu.Lock()
	b.cmdSubs = append(b.cmdSubs, ch)
	b.mu.Unlock()
	return ch
}

// SubscribeEvents returns a channel receiving every published event.
func (b *Bus) SubscribeEvents() <-chan domain.Event {
	ch := make(chan domain.Event, b.buffer)
	b.mu.Lock()
	b.evtSubs = append(b.evtSubs, ch)
	b.mu.Unlock()
	return ch
}

// PublishCommand delivers a command to all command subscribers. Non-blocking:
// a full subscriber channel drops the message rather than stalling the bus.
func (b *Bus) PublishCommand(c domain.Command) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.cmdSubs {
		select {
		case ch <- c:
		default:
		}
	}
}

// PublishEvent delivers an event to all event subscribers (non-blocking).
func (b *Bus) PublishEvent(e domain.Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.evtSubs {
		select {
		case ch <- e:
		default:
		}
	}
}
