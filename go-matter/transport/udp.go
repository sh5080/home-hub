package transport

import (
	"context"
	"fmt"
	"net"
	"time"
)

// MatterPort is the standard Matter operational UDP port.
const MatterPort = 5540

// maxDatagram bounds a received datagram. Matter messages fit within the IPv6
// minimum MTU payload (1280 bytes); a little headroom is added.
const maxDatagram = 1440

// UDP is a Transport over UDP toward a fixed peer address.
type UDP struct {
	conn *net.UDPConn
	peer *net.UDPAddr
}

// ListenUDP opens a UDP socket bound to localAddr ("" or ":0" for an ephemeral
// port, e.g. "[::]:5540" to receive on the Matter port).
func ListenUDP(localAddr string) (*UDP, error) {
	var la *net.UDPAddr
	if localAddr != "" {
		var err error
		if la, err = net.ResolveUDPAddr("udp", localAddr); err != nil {
			return nil, err
		}
	}
	conn, err := net.ListenUDP("udp", la)
	if err != nil {
		return nil, err
	}
	return &UDP{conn: conn}, nil
}

// SetPeer sets the destination address for Send (host:port).
func (u *UDP) SetPeer(addr string) error {
	p, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	u.peer = p
	return nil
}

// LocalAddr returns the socket's local address.
func (u *UDP) LocalAddr() string { return u.conn.LocalAddr().String() }

// Send transmits a datagram to the configured peer.
func (u *UDP) Send(datagram []byte) error {
	if u.peer == nil {
		return fmt.Errorf("transport: UDP peer not set")
	}
	_, err := u.conn.WriteToUDP(datagram, u.peer)
	return err
}

// Receive blocks for the next datagram, honoring ctx cancellation/deadline.
func (u *UDP) Receive(ctx context.Context) ([]byte, error) {
	// A watcher unblocks a pending read when ctx is cancelled by setting an
	// immediate read deadline.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			_ = u.conn.SetReadDeadline(time.Now())
		case <-stop:
		}
	}()
	_ = u.conn.SetReadDeadline(time.Time{}) // clear any prior deadline

	buf := make([]byte, maxDatagram)
	n, _, err := u.conn.ReadFromUDP(buf)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	return append([]byte(nil), buf[:n]...), nil
}

// Close closes the socket.
func (u *UDP) Close() error { return u.conn.Close() }
