// Package ash implements the Asynchronous Serial Host protocol (ASH v2), the
// data-link layer between a host and a Silicon Labs EmberZNet NCP such as the
// Sonoff ZBDongle-E (EFR32MG21). ASH carries EZSP frames over a serial line
// with byte framing, escaping, a CRC, data randomization, and a sliding-window
// ack/retransmit scheme.
//
// This is the foundation for the EZSP Zigbee backend (the "E" dongle), parallel
// to the shimmeringbee/zstack backend used for the "P" dongle. Reference:
// Silicon Labs UG101 "UART Gateway Protocol Reference".
package ash

import "errors"

// Reserved bytes: within a frame body these are byte-stuffed; 0x7E terminates a
// frame and is never escaped inside one.
const (
	Flag   = 0x7E // frame delimiter (end of frame)
	Escape = 0x7D // escape byte
	xon    = 0x11
	xoff   = 0x13
	subst  = 0x18
	cancel = 0x1A // discard the partial frame received so far
)

var (
	ErrShort   = errors.New("ash: frame too short")
	ErrCRC     = errors.New("ash: bad CRC")
	ErrControl = errors.New("ash: unknown control byte")
)

// Type is the ASH frame kind, derived from the control byte.
type Type int

const (
	DATA Type = iota
	ACK
	NAK
	RST    // host -> NCP: reset request
	RSTACK // NCP -> host: reset acknowledgement
	ERROR  // NCP -> host: fatal error
)

// Frame is a decoded ASH frame.
type Frame struct {
	Type    Type
	FrmNum  uint8  // DATA: send sequence 0-7
	AckNum  uint8  // DATA/ACK/NAK: next expected receive sequence 0-7
	ReTx    bool   // DATA: set when this is a retransmission
	Payload []byte // DATA: de-randomized EZSP bytes
	Code    byte   // RSTACK/ERROR: reset/error code
}

// crc16 computes the CCITT-16 CRC (poly 0x1021, init 0xFFFF, no reflection)
// used by ASH over the control byte and data field.
func crc16(data []byte) uint16 {
	crc := uint16(0xFFFF)
	for _, b := range data {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// randomize XORs data with the ASH pseudo-random sequence (seed 0x42). It is
// its own inverse, so the same call de-randomizes a received data field. Only
// DATA-frame payloads are randomized.
func randomize(data []byte) []byte {
	out := make([]byte, len(data))
	r := byte(0x42)
	for i, b := range data {
		out[i] = b ^ r
		if r&1 == 0 {
			r >>= 1
		} else {
			r = r>>1 ^ 0xB8
		}
	}
	return out
}

func isReserved(b byte) bool {
	switch b {
	case Flag, Escape, xon, xoff, subst, cancel:
		return true
	}
	return false
}

// stuff byte-stuffs reserved bytes: a reserved byte becomes Escape followed by
// the byte XOR 0x20.
func stuff(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for _, b := range data {
		if isReserved(b) {
			out = append(out, Escape, b^0x20)
		} else {
			out = append(out, b)
		}
	}
	return out
}

// unstuff reverses stuff.
func unstuff(data []byte) []byte {
	out := make([]byte, 0, len(data))
	esc := false
	for _, b := range data {
		switch {
		case esc:
			out = append(out, b^0x20)
			esc = false
		case b == Escape:
			esc = true
		default:
			out = append(out, b)
		}
	}
	return out
}

// encode builds a complete on-wire frame from a control+data body: appends the
// CRC (big-endian), byte-stuffs, and terminates with the flag byte.
func encode(body []byte) []byte {
	crc := crc16(body)
	full := append(append([]byte{}, body...), byte(crc>>8), byte(crc))
	return append(stuff(full), Flag)
}

// DataFrame builds a DATA frame carrying an EZSP payload. frmNum is the send
// sequence (0-7), ackNum the next receive sequence the host expects (0-7).
func DataFrame(frmNum, ackNum uint8, ezsp []byte) []byte {
	ctrl := frmNum&0x07<<4 | ackNum&0x07 // bit7=0 (DATA), reTx=0
	return encode(append([]byte{ctrl}, randomize(ezsp)...))
}

// AckFrame acknowledges receipt up to ackNum.
func AckFrame(ackNum uint8) []byte { return encode([]byte{0x80 | ackNum&0x07}) }

// NakFrame requests retransmission from ackNum.
func NakFrame(ackNum uint8) []byte { return encode([]byte{0xA0 | ackNum&0x07}) }

// RstFrame resets the NCP. On the wire it is usually preceded by a Cancel byte
// to flush any partial frame; callers can prepend one.
func RstFrame() []byte { return encode([]byte{0xC0}) }

// Decode parses one raw frame body (the stuffed bytes between two flag bytes,
// WITHOUT the trailing flag). It unstuffs, verifies the CRC, and interprets the
// control byte.
func Decode(raw []byte) (Frame, error) {
	b := unstuff(raw)
	if len(b) < 3 { // 1 control + 2 CRC minimum
		return Frame{}, ErrShort
	}
	body := b[:len(b)-2]
	want := uint16(b[len(b)-2])<<8 | uint16(b[len(b)-1])
	if crc16(body) != want {
		return Frame{}, ErrCRC
	}
	ctrl := body[0]
	switch {
	case ctrl&0x80 == 0: // DATA
		return Frame{
			Type: DATA, FrmNum: ctrl >> 4 & 7, ReTx: ctrl&0x08 != 0,
			AckNum: ctrl & 7, Payload: randomize(body[1:]),
		}, nil
	case ctrl&0xE0 == 0x80: // 100xxxxx ACK
		return Frame{Type: ACK, AckNum: ctrl & 7}, nil
	case ctrl&0xE0 == 0xA0: // 101xxxxx NAK
		return Frame{Type: NAK, AckNum: ctrl & 7}, nil
	case ctrl == 0xC0:
		return Frame{Type: RST}, nil
	case ctrl == 0xC1: // RSTACK: ctrl, version, code
		f := Frame{Type: RSTACK}
		if len(body) >= 3 {
			f.Code = body[2]
		}
		return f, nil
	case ctrl == 0xC2: // ERROR: ctrl, version, code
		f := Frame{Type: ERROR}
		if len(body) >= 3 {
			f.Code = body[2]
		}
		return f, nil
	}
	return Frame{}, ErrControl
}
