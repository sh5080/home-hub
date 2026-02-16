package message

import (
	"encoding/binary"
	"fmt"
)

// Exchange (protocol header) flag bits (Spec 4.4.3.1).
const (
	exFlagInitiator  = 1 << 0 // I: message initiates the exchange
	exFlagAck        = 1 << 1 // A: carries an acknowledgement
	exFlagReliable   = 1 << 2 // R: sender requests an MRP acknowledgement
	exFlagSecuredExt = 1 << 3 // SX: secured extensions present
	exFlagVendor     = 1 << 4 // V: protocol vendor id present
)

// ProtoHeader is the protocol header (Spec 4.4.3), carried inside the (possibly
// encrypted) message payload. It identifies the exchange and the operation.
type ProtoHeader struct {
	Initiator bool
	Reliable  bool // R: request MRP acknowledgement

	Opcode     byte
	ExchangeID uint16
	ProtocolID uint16

	VendorID      uint16 // valid when VendorPresent
	VendorPresent bool

	AckCounter uint32 // valid when AckPresent
	AckPresent bool
}

// Encode serializes the protocol header.
func (p *ProtoHeader) Encode() []byte {
	var flags byte
	if p.Initiator {
		flags |= exFlagInitiator
	}
	if p.AckPresent {
		flags |= exFlagAck
	}
	if p.Reliable {
		flags |= exFlagReliable
	}
	if p.VendorPresent {
		flags |= exFlagVendor
	}

	b := make([]byte, 0, 12)
	b = append(b, flags, p.Opcode)
	b = binary.LittleEndian.AppendUint16(b, p.ExchangeID)
	b = binary.LittleEndian.AppendUint16(b, p.ProtocolID)
	if p.VendorPresent {
		b = binary.LittleEndian.AppendUint16(b, p.VendorID)
	}
	if p.AckPresent {
		b = binary.LittleEndian.AppendUint32(b, p.AckCounter)
	}
	return b
}

// DecodeProto parses a protocol header and returns the application payload.
func DecodeProto(data []byte) (ProtoHeader, []byte, error) {
	if len(data) < 6 {
		return ProtoHeader{}, nil, fmt.Errorf("message: protocol header truncated")
	}
	var p ProtoHeader
	flags := data[0]
	p.Initiator = flags&exFlagInitiator != 0
	p.Reliable = flags&exFlagReliable != 0
	p.Opcode = data[1]
	p.ExchangeID = binary.LittleEndian.Uint16(data[2:])
	p.ProtocolID = binary.LittleEndian.Uint16(data[4:])

	pos := 6
	if flags&exFlagVendor != 0 {
		if len(data)-pos < 2 {
			return ProtoHeader{}, nil, fmt.Errorf("message: vendor id truncated")
		}
		p.VendorID = binary.LittleEndian.Uint16(data[pos:])
		p.VendorPresent = true
		pos += 2
	}
	if flags&exFlagAck != 0 {
		if len(data)-pos < 4 {
			return ProtoHeader{}, nil, fmt.Errorf("message: ack counter truncated")
		}
		p.AckCounter = binary.LittleEndian.Uint32(data[pos:])
		p.AckPresent = true
		pos += 4
	}
	if flags&exFlagSecuredExt != 0 {
		return ProtoHeader{}, nil, fmt.Errorf("message: secured extensions not supported")
	}
	return p, data[pos:], nil
}
