package controller

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/sh5080/go-matter/discovery"
)

// Resolve locates a commissioned peer on the local network by computing its
// operational instance name (compressed-fabric-id + node-id) and querying mDNS.
func (c *Controller) Resolve(ctx context.Context, nodeID uint64) (discovery.Node, error) {
	cfid, err := discovery.CompressedFabricID(c.fabric.RootPubKey, c.fabric.FabricID)
	if err != nil {
		return discovery.Node{}, err
	}
	return discovery.Discover(ctx, discovery.OperationalInstanceName(cfid, nodeID))
}

// Dial resolves nodeID over mDNS and establishes a CASE session to it. This is
// the address-free counterpart of DialAddr for devices already commissioned to
// this fabric.
func (c *Controller) Dial(ctx context.Context, nodeID uint64) (*Session, error) {
	node, err := c.Resolve(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	addr, err := pickAddr(node)
	if err != nil {
		return nil, err
	}
	return c.DialAddr(ctx, nodeID, addr)
}

// pickAddr chooses a dialable host:port from a resolved node, preferring a
// routable (global/ULA) address over a link-local one.
//
// LIMITATION: mDNS AAAA records frequently carry only link-local (fe80::)
// addresses, which need a zone id (%interface) to route. The DNS response does
// not include the zone — it is implied by the arriving interface, which
// discovery.Discover currently drops. A link-local-only node therefore will
// not dial until zone tracking is added; we still return it as a last resort
// so the failure surfaces at Dial with a real address rather than here.
func pickAddr(node discovery.Node) (string, error) {
	if len(node.Addrs) == 0 {
		return "", fmt.Errorf("controller: node %q has no addresses", node.Target)
	}
	best := node.Addrs[0]
	for _, a := range node.Addrs {
		if !a.IsLinkLocalUnicast() {
			best = a
			break
		}
	}
	return net.JoinHostPort(best.String(), strconv.Itoa(int(node.Port))), nil
}
