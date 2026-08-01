package ezsp

import (
	"encoding/binary"
	"fmt"
)

// EmberStatus values we act on. The full enum is large; these are the ones the
// bring-up path distinguishes.
const (
	StatusSuccess   = 0x00
	StatusNotJoined = 0x93 // the NCP holds no network — form one (M2)
)

// Node types reported by getNetworkParameters.
const (
	NodeTypeUnknown     = 0x00
	NodeTypeCoordinator = 0x01
	NodeTypeRouter      = 0x02
	NodeTypeEndDevice   = 0x03
	NodeTypeSleepy      = 0x04
)

// NetworkParameters is EmberNetworkParameters as returned by
// getNetworkParameters: the coordinator's current radio and PAN settings.
type NetworkParameters struct {
	ExtendedPanID [8]byte
	PanID         uint16
	RadioTxPower  uint8
	RadioChannel  uint8
	JoinMethod    uint8
	NwkManagerID  uint16
	NwkUpdateID   uint8
	Channels      uint32 // channel mask the network may move within
}

// netParamsLen is the wire size of EmberNetworkParameters: 8+2+1+1+1+2+1+4.
const netParamsLen = 20

// NetworkState is the decoded getNetworkParameters response.
type NetworkState struct {
	Status   uint8
	NodeType uint8
	Params   NetworkParameters
}

// Joined reports whether the NCP is currently on a network. A ZBDongle-E out of
// the box answers NOT_JOINED, which is the signal to form a network (M2) rather
// than an error.
func (s NetworkState) Joined() bool { return s.Status == StatusSuccess }

// DecodeNetworkParameters parses a getNetworkParameters response payload:
// [status][nodeType][EmberNetworkParameters].
func DecodeNetworkParameters(p []byte) (NetworkState, error) {
	if len(p) < 2+netParamsLen {
		return NetworkState{}, fmt.Errorf("ezsp: getNetworkParameters payload too short (%d, want %d)", len(p), 2+netParamsLen)
	}
	s := NetworkState{Status: p[0], NodeType: p[1]}
	b := p[2:]
	copy(s.Params.ExtendedPanID[:], b[0:8])
	s.Params.PanID = binary.LittleEndian.Uint16(b[8:10])
	s.Params.RadioTxPower = b[10]
	s.Params.RadioChannel = b[11]
	s.Params.JoinMethod = b[12]
	s.Params.NwkManagerID = binary.LittleEndian.Uint16(b[13:15])
	s.Params.NwkUpdateID = b[15]
	s.Params.Channels = binary.LittleEndian.Uint32(b[16:20])
	return s, nil
}

// NodeTypeName renders an EmberNodeType for logs.
func NodeTypeName(t uint8) string {
	switch t {
	case NodeTypeUnknown:
		return "unknown"
	case NodeTypeCoordinator:
		return "coordinator"
	case NodeTypeRouter:
		return "router"
	case NodeTypeEndDevice:
		return "end-device"
	case NodeTypeSleepy:
		return "sleepy-end-device"
	}
	return fmt.Sprintf("%#02x", t)
}
