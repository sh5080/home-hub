package discovery

import (
	"net/netip"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

func TestBuildCommissionableQuery(t *testing.T) {
	packet, err := BuildCommissionableQuery(3840)
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
	want := "_L3840._sub._matterc._udp.local."
	if q.Type != dnsmessage.TypePTR || q.Name.String() != want {
		t.Fatalf("question = %+v, want PTR %s", q, want)
	}
}

func synthCommissionable(t *testing.T, instance, host string, port uint16, addr netip.Addr) []byte {
	t.Helper()
	subtype, _ := dnsmessage.NewName("_L3840._sub._matterc._udp.local.")
	instName, _ := dnsmessage.NewName(instance + "._matterc._udp.local.")
	target, _ := dnsmessage.NewName(host)

	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{Response: true, Authoritative: true})
	if err := b.StartAnswers(); err != nil {
		t.Fatal(err)
	}
	if err := b.PTRResource(
		dnsmessage.ResourceHeader{Name: subtype, Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET, TTL: 120},
		dnsmessage.PTRResource{PTR: instName},
	); err != nil {
		t.Fatal(err)
	}
	if err := b.SRVResource(
		dnsmessage.ResourceHeader{Name: instName, Type: dnsmessage.TypeSRV, Class: dnsmessage.ClassINET, TTL: 120},
		dnsmessage.SRVResource{Port: port, Target: target},
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

func TestParseCommissionableResponse(t *testing.T) {
	addr := netip.MustParseAddr("fe80::abcd")
	packet := synthCommissionable(t, "A1B2C3D4E5F60718", "commissionee.local.", 5540, addr)

	c, err := ParseCommissionableResponse(packet)
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != 5540 || c.Target != "commissionee.local." {
		t.Fatalf("commissionable = %+v", c)
	}
	if c.Instance != "A1B2C3D4E5F60718._matterc._udp.local." {
		t.Fatalf("instance = %q", c.Instance)
	}
	if len(c.Addrs) != 1 || c.Addrs[0] != addr {
		t.Fatalf("addrs = %v", c.Addrs)
	}
}

func TestParseCommissionableNoSRV(t *testing.T) {
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{Response: true})
	b.StartAnswers()
	packet, _ := b.Finish()
	if _, err := ParseCommissionableResponse(packet); err == nil {
		t.Fatal("expected error when no SRV record is present")
	}
}
