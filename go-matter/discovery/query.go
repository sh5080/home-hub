package discovery

import (
	"fmt"
	"net/netip"

	"golang.org/x/net/dns/dnsmessage"
)

// operationalService is the DNS-SD service for commissioned Matter nodes.
const operationalService = "_matter._tcp.local."

// Node is a resolved operational device endpoint.
type Node struct {
	Target string       // SRV target hostname
	Port   uint16       // operational UDP port (usually 5540)
	Addrs  []netip.Addr // A/AAAA addresses of Target
}

// operationalFQDN is the fully-qualified instance name queried in mDNS.
func operationalFQDN(instanceName string) string {
	return instanceName + "." + operationalService
}

// BuildQuery builds an mDNS SRV query for a node's operational instance.
func BuildQuery(instanceName string) ([]byte, error) {
	name, err := dnsmessage.NewName(operationalFQDN(instanceName))
	if err != nil {
		return nil, err
	}
	msg := dnsmessage.Message{
		Header: dnsmessage.Header{RecursionDesired: false},
		Questions: []dnsmessage.Question{{
			Name:  name,
			Type:  dnsmessage.TypeSRV,
			Class: dnsmessage.ClassINET,
		}},
	}
	return msg.Pack()
}

// ParseResponse extracts the operational endpoint (port and addresses) for
// instanceName from a DNS-SD response packet.
func ParseResponse(instanceName string, packet []byte) (Node, error) {
	var node Node
	want := operationalFQDN(instanceName)

	var p dnsmessage.Parser
	if _, err := p.Start(packet); err != nil {
		return node, err
	}
	if err := p.SkipAllQuestions(); err != nil {
		return node, err
	}

	for {
		h, err := p.AnswerHeader()
		if err == dnsmessage.ErrSectionDone {
			break
		}
		if err != nil {
			return node, err
		}
		switch h.Type {
		case dnsmessage.TypeSRV:
			srv, err := p.SRVResource()
			if err != nil {
				return node, err
			}
			if h.Name.String() == want {
				node.Port = srv.Port
				node.Target = srv.Target.String()
			}
		case dnsmessage.TypeAAAA:
			aaaa, err := p.AAAAResource()
			if err != nil {
				return node, err
			}
			if addr, ok := netip.AddrFromSlice(aaaa.AAAA[:]); ok {
				node.Addrs = append(node.Addrs, addr)
			}
		case dnsmessage.TypeA:
			a, err := p.AResource()
			if err != nil {
				return node, err
			}
			node.Addrs = append(node.Addrs, netip.AddrFrom4(a.A))
		default:
			if err := p.SkipAnswer(); err != nil {
				return node, err
			}
		}
	}
	if node.Port == 0 {
		return node, fmt.Errorf("discovery: no SRV record for %s", want)
	}
	return node, nil
}
