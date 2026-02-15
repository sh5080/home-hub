// Package message encodes and decodes the Matter message frame: the plaintext
// message header (Spec 4.4.1) and the protocol header (Spec 4.4.3) that carries
// the exchange and opcode.
//
// For a secured session the protocol header and payload are encrypted and the
// message header authenticates them as additional data (AAD). This package only
// handles plaintext layout; encryption belongs to the session package.
package message

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// SessionType is the security session type (Spec 4.4.1.2).
type SessionType uint8

const (
	Unicast SessionType = 0
	Group   SessionType = 1
)

// DestKind indicates which destination field is present (the DSIZ field).
type DestKind uint8

const (
	DestNone  DestKind = 0 // no destination node/group id
	DestNode  DestKind = 1 // 64-bit destination node id
	DestGroup DestKind = 2 // 16-bit destination group id
)

// Header is the plaintext Matter message header (Spec 4.4.1). Its exact byte
// layout is preserved across decode/encode because it is used as AEAD
// additional data for secured messages.
type Header struct {
	SessionID   uint16
	SessionType SessionType
	Privacy     bool // P: privacy obfuscation applied to the message
	Control     bool // C: control message

	Counter uint32 // per-session message counter

	SourceNodeID  uint64 // valid when SourcePresent
	SourcePresent bool

	DestNodeID  uint64   // valid when DestKind == DestNode
	DestGroupID uint16   // valid when DestKind == DestGroup
	DestKind    DestKind
}

const headerVersion = 0

// Message flag bits (Spec 4.4.1.1): bits 4-7 version, bit 2 S, bits 0-1 DSIZ.
const (
	flagVersionShift = 4
	flagVersionMask  = 0xF0
	flagSource       = 1 << 2
	flagDSIZMask     = 0x03
)

// Security flag bits (Spec 4.4.1.2): bit 7 P, bit 6 C, bit 5 MX, bits 0-1 type.
const (
	secPrivacy         = 1 << 7
	secControl         = 1 << 6
	secExtensions      = 1 << 5
	secSessionTypeMask = 0x03
)

var errShortHeader = errors.New("message: header truncated")

// Encode serializes the message header.
func (h *Header) Encode() ([]byte, error) {
	if h.DestKind > DestGroup {
		return nil, fmt.Errorf("message: invalid destination kind %d", h.DestKind)
	}
	msgFlags := byte(headerVersion << flagVersionShift)
	if h.SourcePresent {
		msgFlags |= flagSource
	}
	msgFlags |= byte(h.DestKind) & flagDSIZMask

	secFlags := byte(h.SessionType) & secSessionTypeMask
	if h.Privacy {
		secFlags |= secPrivacy
	}
	if h.Control {
		secFlags |= secControl
	}

	b := make([]byte, 0, 24)
	b = append(b, msgFlags)
	b = binary.LittleEndian.AppendUint16(b, h.SessionID)
	b = append(b, secFlags)
	b = binary.LittleEndian.AppendUint32(b, h.Counter)
	if h.SourcePresent {
		b = binary.LittleEndian.AppendUint64(b, h.SourceNodeID)
	}
	switch h.DestKind {
	case DestNode:
		b = binary.LittleEndian.AppendUint64(b, h.DestNodeID)
	case DestGroup:
		b = binary.LittleEndian.AppendUint16(b, h.DestGroupID)
	}
	return b, nil
}

// Decode parses a message header and returns the remaining bytes (the protocol
// header + payload, which may still be encrypted).
func Decode(data []byte) (Header, []byte, error) {
	if len(data) < 8 {
		return Header{}, nil, errShortHeader
	}
	var h Header
	msgFlags := data[0]
	if v := (msgFlags & flagVersionMask) >> flagVersionShift; v != headerVersion {
		return Header{}, nil, fmt.Errorf("message: unsupported version %d", v)
	}
	h.SessionID = binary.LittleEndian.Uint16(data[1:])
	secFlags := data[3]
	h.Counter = binary.LittleEndian.Uint32(data[4:])
	h.SessionType = SessionType(secFlags & secSessionTypeMask)
	h.Privacy = secFlags&secPrivacy != 0
	h.Control = secFlags&secControl != 0

	if secFlags&secExtensions != 0 {
		// Message extensions are unused by controllers; reject rather than
		// risk misparsing the payload boundary.
		return Header{}, nil, errors.New("message: message extensions not supported")
	}

	pos := 8
	if msgFlags&flagSource != 0 {
		if len(data)-pos < 8 {
			return Header{}, nil, errShortHeader
		}
		h.SourceNodeID = binary.LittleEndian.Uint64(data[pos:])
		h.SourcePresent = true
		pos += 8
	}
	switch DestKind(msgFlags & flagDSIZMask) {
	case DestNone:
	case DestNode:
		if len(data)-pos < 8 {
			return Header{}, nil, errShortHeader
		}
		h.DestNodeID = binary.LittleEndian.Uint64(data[pos:])
		h.DestKind = DestNode
		pos += 8
	case DestGroup:
		if len(data)-pos < 2 {
			return Header{}, nil, errShortHeader
		}
		h.DestGroupID = binary.LittleEndian.Uint16(data[pos:])
		h.DestKind = DestGroup
		pos += 2
	default:
		return Header{}, nil, errors.New("message: reserved DSIZ value")
	}
	return h, data[pos:], nil
}
