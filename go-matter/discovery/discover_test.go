package discovery

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"
)

// fakeConn hands back queued packets, then emulates a real socket by blocking
// until the read deadline and returning a timeout.
type fakeConn struct {
	responses [][]byte
	deadline  time.Time
	writes    int
	zone      string // source-address zone reported by ReadFrom
}

func (f *fakeConn) WriteTo(b []byte, _ net.Addr) (int, error) { f.writes++; return len(b), nil }

func (f *fakeConn) ReadFrom(b []byte) (int, net.Addr, error) {
	if len(f.responses) == 0 {
		if d := time.Until(f.deadline); d > 0 {
			time.Sleep(d)
		}
		return 0, nil, timeoutErr{}
	}
	pkt := f.responses[0]
	f.responses = f.responses[1:]
	n := copy(b, pkt)
	src := &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: 5353, Zone: f.zone}
	return n, src, nil
}

func (f *fakeConn) SetReadDeadline(t time.Time) error { f.deadline = t; return nil }
func (f *fakeConn) Close() error                      { return nil }

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestDiscoverMatch(t *testing.T) {
	instance := "2906C908D115D362-CD5544AA7B13EF14"
	addr := netip.MustParseAddr("fe80::1234:5678:9abc:def0")
	conn := &fakeConn{
		responses: [][]byte{synthResponse(t, instance, "device1234.local.", 5540, addr)},
		zone:      "en0",
	}

	node, err := discover(context.Background(), conn, instance, mdnsIPv4)
	if err != nil {
		t.Fatal(err)
	}
	if node.Port != 5540 || node.Target != "device1234.local." {
		t.Fatalf("node = %+v", node)
	}
	if conn.writes < 1 {
		t.Fatal("query was never sent")
	}
	// The link-local AAAA must be scoped with the arriving interface's zone,
	// otherwise it cannot be dialed.
	if len(node.Addrs) != 1 || node.Addrs[0].Zone() != "en0" {
		t.Fatalf("addr zone not attached: %v", node.Addrs)
	}
}

func TestAttachZoneSkipsRoutable(t *testing.T) {
	// A global address must not be given a zone.
	node := Node{Addrs: []netip.Addr{netip.MustParseAddr("2001:db8::1")}}
	attachZone(&node, &net.UDPAddr{Zone: "en0"})
	if node.Addrs[0].Zone() != "" {
		t.Fatal("routable address must not be zoned")
	}
}

func TestDiscoverSkipsUnrelated(t *testing.T) {
	instance := "2906C908D115D362-CD5544AA7B13EF14"
	other := synthResponse(t, "1111111111111111-2222222222222222", "other.local.", 1234, netip.MustParseAddr("fe80::9"))
	match := synthResponse(t, instance, "dev.local.", 5540, netip.MustParseAddr("fe80::1"))
	conn := &fakeConn{responses: [][]byte{other, match}}

	node, err := discover(context.Background(), conn, instance, mdnsIPv4)
	if err != nil {
		t.Fatal(err)
	}
	if node.Target != "dev.local." {
		t.Fatalf("resolved wrong node: %+v", node)
	}
}

func TestDiscoverTimeout(t *testing.T) {
	conn := &fakeConn{} // never answers
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()

	if _, err := discover(ctx, conn, "2906C908D115D362-CD5544AA7B13EF14", mdnsIPv4); err == nil {
		t.Fatal("expected a timeout error when no device answers")
	}
}
