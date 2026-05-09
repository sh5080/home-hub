package discovery

import (
	"net/netip"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

func TestBuildQuery(t *testing.T) {
	instance := "2906C908D115D362-CD5544AA7B13EF14"
	packet, err := BuildQuery(instance)
	if err != nil {
		t.Fatal(err)
	}
	var p dnsmessage.Parser
	if _, err := p.Start(packet); err != nil {
		t.Fatal(err)
	}
	q, err := p.Question()
	if err != nil {
		t.Fatal(err)
	}
	if q.Type != dnsmessage.TypeSRV || q.Name.String() != instance+"._matter._tcp.local." {
		t.Fatalf("question = %+v", q)
	}
}

// synthResponse builds a DNS-SD response for instance with the given port and
// address, as a device would answer.
func synthResponse(t *testing.T, instance, host string, port uint16, addr netip.Addr) []byte {
	t.Helper()
	instName, _ := dnsmessage.NewName(instance + "._matter._tcp.local.")
	target, _ := dnsmessage.NewName(host)
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{Response: true, Authoritative: true})
	if err := b.StartAnswers(); err != nil {
		t.Fatal(err)
	}
	if err := b.SRVResource(
		dnsmessage.ResourceHeader{Name: instName, Type: dnsmessage.TypeSRV, Class: dnsmessage.ClassINET, TTL: 120},
		dnsmessage.SRVResource{Priority: 0, Weight: 0, Port: port, Target: target},
	); err != nil {
		t.Fatal(err)
	}
	a16 := addr.As16()
	if err := b.AAAAResource(
		dnsmessage.ResourceHeader{Name: target, Type: dnsmessage.TypeAAAA, Class: dnsmessage.ClassINET, TTL: 120},
		dnsmessage.AAAAResource{AAAA: a16},
	); err != nil {
		t.Fatal(err)
	}
	packet, err := b.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return packet
}

func TestParseResponse(t *testing.T) {
	instance := "2906C908D115D362-CD5544AA7B13EF14"
	addr := netip.MustParseAddr("fe80::1234:5678:9abc:def0")
	packet := synthResponse(t, instance, "device1234.local.", 5540, addr)

	node, err := ParseResponse(instance, packet)
	if err != nil {
		t.Fatal(err)
	}
	if node.Port != 5540 {
		t.Fatalf("port = %d", node.Port)
	}
	if node.Target != "device1234.local." {
		t.Fatalf("target = %q", node.Target)
	}
	if len(node.Addrs) != 1 || node.Addrs[0] != addr {
		t.Fatalf("addrs = %v", node.Addrs)
	}
}

func TestParseResponseNoSRV(t *testing.T) {
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{Response: true})
	b.StartAnswers()
	packet, _ := b.Finish()
	if _, err := ParseResponse("x-y", packet); err == nil {
		t.Fatal("expected error when no SRV record is present")
	}
}
