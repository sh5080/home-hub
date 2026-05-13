package controller

import (
	"context"

	"github.com/sh5080/go-matter/transport"
)

// DialAddr opens a UDP transport to a device at addr (host:port, typically the
// device address and port 5540) and performs CASE toward nodeID. The returned
// Session owns the transport and must be Closed when done.
func (c *Controller) DialAddr(ctx context.Context, nodeID uint64, addr string) (*Session, error) {
	udp, err := transport.ListenUDP("")
	if err != nil {
		return nil, err
	}
	if err := udp.SetPeer(addr); err != nil {
		udp.Close()
		return nil, err
	}
	sess, err := c.Connect(ctx, udp, nodeID)
	if err != nil {
		udp.Close()
		return nil, err
	}
	return sess, nil
}

// Close releases the session's transport.
func (s *Session) Close() error { return s.t.Close() }
