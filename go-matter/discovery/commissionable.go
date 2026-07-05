package discovery

import (
	"fmt"
	"net/netip"

	"golang.org/x/net/dns/dnsmessage"
)

// commissionableService is the DNS-SD service for devices in commissioning
// mode (Spec 4.3.1). Unlike operational nodes it advertises over _udp.
const commissionableService = "_matterc._udp.local."

// longDiscriminatorSubtype returns the browse name for a 12-bit long
// discriminator: _L<disc>._sub._matterc._udp.local.
func longDiscriminatorSubtype(discriminator uint16) string {
	return fmt.Sprintf("_L%d._sub.%s", discriminator, commissionableService)
}

// BuildCommissionableQuery builds a PTR query browsing for a commissionable
// device with the given 12-bit long discriminator.
func BuildCommissionableQuery(discriminator uint16) ([]byte, error) {
	name, err := dnsmessage.NewName(longDiscriminatorSubtype(discriminator))
	if err != nil {
		return nil, err
	}
	msg := dnsmessage.Message{
		Header: dnsmessage.Header{RecursionDesired: false},
		Questions: []dnsmessage.Question{{
			Name:  name,
			Type:  dnsmessage.TypePTR,
			Class: dnsmessage.ClassINET,
		}},
	}
	return msg.Pack()
}

// Commissionable is a discovered device in commissioning mode.
type Commissionable struct {
	Instance string       // service instance (the device's random name)
	Target   string       // SRV target hostname
	Port     uint16       // commissioning UDP port
	Addrs    []netip.Addr // A/AAAA addresses of Target
}

// ParseCommissionableResponse extracts the endpoint for a commissionable
// device from a DNS-SD response: PTR (subtype→instance) + SRV + A/AAAA.
func ParseCommissionableResponse(packet []byte) (Commissionable, error) {
	var c Commissionable

	var p dnsmessage.Parser
	if _, err := p.Start(packet); err != nil {
		return c, err
	}
	if err := p.SkipAllQuestions(); err != nil {
		return c, err
	}

	for {
		h, err := p.AnswerHeader()
		if err == dnsmessage.ErrSectionDone {
			break
		}
		if err != nil {
			return c, err
		}
		switch h.Type {
		case dnsmessage.TypePTR:
			ptr, err := p.PTRResource()
			if err != nil {
				return c, err
			}
			// Record the pointed-to instance (strip the trailing service).
			c.Instance = ptr.PTR.String()
		case dnsmessage.TypeSRV:
			srv, err := p.SRVResource()
			if err != nil {
				return c, err
			}
			c.Port = srv.Port
			c.Target = srv.Target.String()
		case dnsmessage.TypeAAAA:
			aaaa, err := p.AAAAResource()
			if err != nil {
				return c, err
			}
			if addr, ok := netip.AddrFromSlice(aaaa.AAAA[:]); ok {
				c.Addrs = append(c.Addrs, addr)
			}
		case dnsmessage.TypeA:
			a, err := p.AResource()
			if err != nil {
				return c, err
			}
			c.Addrs = append(c.Addrs, netip.AddrFrom4(a.A))
		default:
			if err := p.SkipAnswer(); err != nil {
				return c, err
			}
		}
	}
	if c.Port == 0 {
		return c, fmt.Errorf("discovery: no SRV record in commissionable response")
	}
	return c, nil
}
