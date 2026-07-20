// Package ezsp implements the EmberZNet Serial Protocol carried over ASH, the
// host side of a Silicon Labs Zigbee NCP (Sonoff ZBDongle-E, EFR32MG21). It is
// the "E" backend, parallel to the shimmeringbee/zstack "P" backend.
//
// EZSP has two on-wire frame formats: the legacy format (EZSP < 8, single-byte
// frame id) used to bootstrap the version handshake, and the extended format
// (EZSP >= 8, two-byte frame id) used once the version is negotiated. This file
// implements the legacy format needed for M0 (connect + version); the extended
// format and the command set beyond version are the next milestone.
package ezsp

import (
	"encoding/binary"
	"fmt"
)

// Frame identifiers (subset). Commands sent host->NCP, callbacks NCP->host.
// VERIFY the extended-format ids against a real NCP when implementing them.
const (
	IDVersion              = 0x0000 // negotiate EZSP version
	IDGetNetworkParameters = 0x0028
	IDNetworkInit          = 0x0017
	IDFormNetwork          = 0x001E
	IDPermitJoining        = 0x0022
	IDSendUnicast          = 0x0034
	IDStackStatusHandler   = 0x0019 // callback: network up/down
	IDTrustCenterJoin      = 0x0024 // callback: a node joined/left
	IDIncomingMessage      = 0x0045 // callback: incoming APS/ZCL message
)

// LegacyCommand builds an EZSP command in the legacy (pre-v8) frame format:
// [sequence][frame control = 0x00 (command)][frame id (1 byte)][parameters].
// Only valid for single-byte frame ids; used to bootstrap the version handshake.
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
