package message

import (
	"encoding/binary"
	"fmt"
)

// General status codes for a StatusReport (Spec 4.9.4).
const (
	GeneralSuccess uint16 = 0x0000
	GeneralFailure uint16 = 0x0001
	GeneralBusy    uint16 = 0x0002
)

// StatusReport is the Secure Channel StatusReport payload (Spec 4.9.4). It is a
// fixed little-endian layout (not TLV) used to signal handshake success or
// failure. A SUCCESS report ends a PASE/CASE handshake.
type StatusReport struct {
	GeneralCode  uint16
	ProtocolID   uint32
	ProtocolCode uint16
}

// Encode serializes the status report (8 bytes).
func (s StatusReport) Encode() []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint16(b[0:], s.GeneralCode)
	binary.LittleEndian.PutUint32(b[2:], s.ProtocolID)
	binary.LittleEndian.PutUint16(b[6:], s.ProtocolCode)
	return b
}

// DecodeStatusReport parses a status report payload.
func DecodeStatusReport(b []byte) (StatusReport, error) {
	if len(b) < 8 {
		return StatusReport{}, fmt.Errorf("message: status report truncated")
	}
	return StatusReport{
		GeneralCode:  binary.LittleEndian.Uint16(b[0:]),
		ProtocolID:   binary.LittleEndian.Uint32(b[2:]),
		ProtocolCode: binary.LittleEndian.Uint16(b[6:]),
	}, nil
}

// IsSuccess reports whether the status indicates success.
func (s StatusReport) IsSuccess() bool { return s.GeneralCode == GeneralSuccess }

// Error renders a non-success status report as an error, or nil on success.
func (s StatusReport) Error() error {
	if s.IsSuccess() {
		return nil
	}
	return fmt.Errorf("status report: general=0x%04x protocol=0x%08x code=0x%04x",
		s.GeneralCode, s.ProtocolID, s.ProtocolCode)
}
