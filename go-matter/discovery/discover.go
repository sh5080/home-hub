package discovery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

// mDNS link-local multicast endpoint (RFC 6762 §3). IPv6 (ff02::fb) is the
// other group; operational discovery works over IPv4 on a typical home LAN,
// and IPv6 support can be layered on the same loop when needed.
var mdnsIPv4 = &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}

// retransmitInterval bounds how long we wait for a response before resending
// the query. mDNS is unreliable multicast, so a lost query must be retried.
const retransmitInterval = time.Second

// packetConn is the subset of net.PacketConn that Discover needs. It is an
// interface so the response loop can be tested without real multicast I/O.
type packetConn interface {
	WriteTo(b []byte, addr net.Addr) (int, error)
	ReadFrom(b []byte) (int, net.Addr, error)
	SetReadDeadline(t time.Time) error
	Close() error
}

// Discover resolves a commissioned node's operational endpoint over mDNS. It
// broadcasts an SRV query for instanceName and returns the first matching Node.
//
// It joins the IPv4 mDNS group on port 5353, so a host already running a mDNS
// responder (avahi, mDNSResponder) that holds 5353 without SO_REUSEPORT may
// prevent binding — acceptable on the appliance-style deployment target, but
// worth noting when debugging "address already in use".
func Discover(ctx context.Context, instanceName string) (Node, error) {
	conn, err := net.ListenMulticastUDP("udp4", nil, mdnsIPv4)
	if err != nil {
		return Node{}, err
	}
	defer conn.Close()
	return discover(ctx, conn, instanceName, mdnsIPv4)
}

// discover runs the query/response loop against an arbitrary packetConn,
// retransmitting until a matching SRV arrives, ctx is cancelled, or the ctx
// deadline (or a bounded default) elapses.
func discover(ctx context.Context, conn packetConn, instanceName string, group net.Addr) (Node, error) {
	query, err := BuildQuery(instanceName)
	if err != nil {
		return Node{}, err
	}

	overall, ok := ctx.Deadline()
	if !ok {
		overall = time.Now().Add(5 * time.Second)
	}
	if _, err := conn.WriteTo(query, group); err != nil {
		return Node{}, err
	}

	buf := make([]byte, 1500)
	for {
		if err := ctx.Err(); err != nil {
			return Node{}, err
		}
		read := time.Now().Add(retransmitInterval)
		if read.After(overall) {
			read = overall
		}
		_ = conn.SetReadDeadline(read)

		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			if isTimeout(err) {
				if !time.Now().Before(overall) {
					return Node{}, fmt.Errorf("discovery: timed out resolving %s", instanceName)
				}
				if _, werr := conn.WriteTo(query, group); werr != nil {
					return Node{}, werr
				}
				continue
			}
			return Node{}, err
		}
		if node, perr := ParseResponse(instanceName, buf[:n]); perr == nil {
			return node, nil
		}
		// Unrelated packet (another service, our own query echoed back) — keep listening.
	}
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}
