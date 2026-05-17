package controller

import (
	"net/netip"
	"testing"

	"github.com/sh5080/go-matter/discovery"
)

func TestPickAddrPrefersRoutable(t *testing.T) {
	node := discovery.Node{
		Target: "dev.local.",
		Port:   5540,
		Addrs: []netip.Addr{
			netip.MustParseAddr("fe80::1"),        // link-local, listed first
			netip.MustParseAddr("2001:db8::1234"), // routable
		},
	}
	got, err := pickAddr(node)
	if err != nil {
		t.Fatal(err)
	}
	if got != "[2001:db8::1234]:5540" {
		t.Fatalf("addr = %q, want the routable address", got)
	}
}

func TestPickAddrFallsBackToLinkLocal(t *testing.T) {
	node := discovery.Node{
		Target: "dev.local.", Port: 5540,
		Addrs: []netip.Addr{netip.MustParseAddr("fe80::abcd")},
	}
	got, err := pickAddr(node)
	if err != nil {
		t.Fatal(err)
	}
	if got != "[fe80::abcd]:5540" {
		t.Fatalf("addr = %q", got)
	}
}

func TestPickAddrNoAddresses(t *testing.T) {
	if _, err := pickAddr(discovery.Node{Target: "dev.local.", Port: 5540}); err == nil {
		t.Fatal("expected an error when the node has no addresses")
	}
}
