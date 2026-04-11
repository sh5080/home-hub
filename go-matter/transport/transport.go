// Package transport carries Matter message datagrams between the controller and
// a device. It abstracts the wire — UDP for real devices, an in-memory pipe for
// tests and loopback — behind a small datagram interface.
package transport

import (
	"context"
	"errors"
	"sync"
)

// ErrClosed is returned once a transport has been closed.
var ErrClosed = errors.New("transport: closed")

// Transport sends and receives Matter message datagrams to/from one peer.
type Transport interface {
	Send(datagram []byte) error
	Receive(ctx context.Context) ([]byte, error)
	Close() error
}

// Pipe is one end of an in-memory bidirectional transport pair.
type Pipe struct {
	send   chan []byte
	recv   chan []byte
	closed chan struct{}
	once   *sync.Once // shared with the peer so either end tears down the pair
}

// NewPipe returns two connected transports: datagrams sent on a are received on
// b and vice versa. Closing either end closes both directions.
func NewPipe() (a, b *Pipe) {
	ab := make(chan []byte, 16)
	ba := make(chan []byte, 16)
	closed := make(chan struct{})
	once := &sync.Once{}
	a = &Pipe{send: ab, recv: ba, closed: closed, once: once}
	b = &Pipe{send: ba, recv: ab, closed: closed, once: once}
	return a, b
}

// Send delivers a copy of datagram to the peer.
func (p *Pipe) Send(datagram []byte) error {
	cp := append([]byte(nil), datagram...)
	select {
	case p.send <- cp:
		return nil
	case <-p.closed:
		return ErrClosed
	}
}

// Receive blocks for the next datagram, until ctx is done or the pipe closes.
func (p *Pipe) Receive(ctx context.Context) ([]byte, error) {
	select {
	case b := <-p.recv:
		return b, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.closed:
		return nil, ErrClosed
	}
}

// Close closes both directions of the pipe. Safe to call multiple times and
// from either end.
func (p *Pipe) Close() error {
	p.once.Do(func() { close(p.closed) })
	return nil
}
