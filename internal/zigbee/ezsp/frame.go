// Package ezsp implements the EmberZNet Serial Protocol carried over ASH, the
// host side of a Silicon Labs Zigbee NCP (Sonoff ZBDongle-E, EFR32MG21). It is
// the "E" backend, parallel to the shimmeringbee/zstack "P" backend.
//
// EZSP has two on-wire frame formats:
//
//   - legacy (EZSP < 8): [seq][frame control 1B][frame id 1B][params]
//   - extended (EZSP >= 8): [seq][frame control 2B LE][frame id 2B LE][params]
//
// The version command is the bootstrap exception: it must be sent in the legacy
// format because the format itself is what the handshake negotiates, and the
// NCP answers it in kind. Every frame after that — in both directions — uses
// the extended format.
//
// Frame ids and the extended layout were cross-checked against zigpy/bellows
// (v8 _ezsp_frame_tx/_rx) and zigbee-herdsman's EzspFrameID enum, then verified
// against a real ZBDongle-E reporting ezspVersion=13.
package ezsp

import (
	"encoding/binary"
	"fmt"
)

// Frame identifiers. Commands are sent host->NCP; handlers are callbacks the
// NCP sends unsolicited. The numeric values are identical in the legacy and
// extended formats — only the on-wire width differs.
const (
	IDVersion                 = 0x0000 // negotiate EZSP version
	IDAddEndpoint             = 0x0002 // register an application endpoint
	IDNop                     = 0x0005 // no-op, useful as a liveness probe
	IDNetworkInit             = 0x0017 // resume the network stored in NCP flash
	IDStackStatusHandler      = 0x0019 // callback: network up/down
	IDFormNetwork             = 0x001E // create a new network as coordinator
	IDLeaveNetwork            = 0x0020 // tear the network down
	IDPermitJoining           = 0x0022 // open the network for joins
	IDTrustCenterJoinHandler  = 0x0024 // callback: a node joined or left
	IDGetNetworkParameters    = 0x0028 // read PAN id / channel / node type
	IDSendUnicast             = 0x0034 // send an APS frame to one node
	IDMessageSentHandler      = 0x003F // callback: unicast delivery result
	IDIncomingMessageHandler  = 0x0045 // callback: incoming APS/ZCL message
	IDSetConfigurationValue   = 0x0053 // NCP table sizes and limits
	IDSetPolicy               = 0x0055 // trust-center / join policies
	IDSetInitialSecurityState = 0x0068 // network key material before forming
)

// frameControlExtended is the frame control this host sends on every extended
// command: 0x0100 little-endian. The high byte's low bits carry the frame
// format version (1 = extended); the low byte is all zero, marking a command
// with no callback or security flags set.
const frameControlExtended = 0x0100

// responseBit is set in a frame control received from the NCP, distinguishing
// its frames from the commands we send. It is recorded for diagnostics only:
// responses are matched to requests by (sequence, frame id), which is what
// bellows does and what tolerates an NCP that sets flags we do not model.
const responseBit = 0x0080

// Command builds an EZSP command in the extended (EZSP >= 8) frame format.
// Every command except the version handshake uses this.
func Command(seq uint8, id uint16, params []byte) []byte {
	b := make([]byte, 5, 5+len(params))
	b[0] = seq
	binary.LittleEndian.PutUint16(b[1:3], frameControlExtended)
	binary.LittleEndian.PutUint16(b[3:5], id)
	return append(b, params...)
}

// Frame is a decoded extended-format EZSP frame received from the NCP.
type Frame struct {
	Seq     uint8
	Control uint16
	ID      uint16
	Params  []byte
}

// IsResponse reports whether the NCP marked this frame as a response. Callbacks
// also carry the bit, so this is not on its own a demultiplexing signal.
func (f Frame) IsResponse() bool { return f.Control&responseBit != 0 }

// DecodeFrame parses an extended-format frame. The frame control is retained
// rather than validated: NCPs set flags (overflow, truncated, callback type)
// that we do not model yet, and rejecting on unknown bits would break the link
// for a condition the caller can simply log.
func DecodeFrame(p []byte) (Frame, error) {
	if len(p) < 5 {
		return Frame{}, fmt.Errorf("ezsp: extended frame too short (%d bytes)", len(p))
	}
	return Frame{
		Seq:     p[0],
		Control: binary.LittleEndian.Uint16(p[1:3]),
		ID:      binary.LittleEndian.Uint16(p[3:5]),
		Params:  p[5:],
	}, nil
}

// LegacyCommand builds an EZSP command in the legacy (pre-v8) frame format:
// [sequence][frame control = 0x00 (command)][frame id (1 byte)][parameters].
// Only the version handshake uses this; see the package comment.
func LegacyCommand(seq uint8, id uint8, params []byte) []byte {
	return append([]byte{seq, 0x00, id}, params...)
}

// VersionCommand asks the NCP to negotiate to desiredVersion (e.g. 13). The NCP
// replies with the highest version it supports.
func VersionCommand(seq, desiredVersion uint8) []byte {
	return LegacyCommand(seq, IDVersion, []byte{desiredVersion})
}

// VersionResponse is the decoded reply to a version command.
type VersionResponse struct {
	ProtocolVersion uint8  // EZSP protocol version the NCP will speak
	StackType       uint8  // 2 = EmberZNet
	StackVersion    uint16 // NCP stack build
}

// DecodeVersionResponse parses a legacy version response payload:
// [seq][frame control=0x80][frame id=0x00][protocol][stackType][stackVersion LE].
func DecodeVersionResponse(p []byte) (VersionResponse, error) {
	if len(p) < 7 {
		return VersionResponse{}, fmt.Errorf("ezsp: version response too short (%d)", len(p))
	}
	if p[2] != IDVersion {
		return VersionResponse{}, fmt.Errorf("ezsp: not a version response (id %#02x)", p[2])
	}
	return VersionResponse{
		ProtocolVersion: p[3],
		StackType:       p[4],
		StackVersion:    binary.LittleEndian.Uint16(p[5:7]),
	}, nil
}
