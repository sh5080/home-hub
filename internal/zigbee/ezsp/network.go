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

// ---------------------------------------------------------------------------
// M2: bringing the coordinator's network up.
// ---------------------------------------------------------------------------

// EmberStatus values seen during network bring-up.
const (
	StatusNetworkUp   = 0x90
	StatusNetworkDown = 0x91
)

// EmberInitialSecurityBitmask flags used when forming.
const (
	SecTrustCenterGlobalLinkKey = 0x0004
	SecTrustCenterHashedLinkKey = 0x0084 // implies the global-link-key bit
	SecHavePreconfiguredKey     = 0x0100
	SecHaveNetworkKey           = 0x0200
	SecRequireEncryptedKey      = 0x0800
)

// formSecurityBitmask is the Zigbee 3.0 coordinator profile: a global trust
// centre link key (hashed), a network key we supply, and the network key handed
// to joiners encrypted rather than in the clear. Mirrors what zigbee-herdsman's
// Ember adapter forms with, so devices that pair with zigbee2mqtt pair with us.
const formSecurityBitmask = SecTrustCenterHashedLinkKey |
	SecHavePreconfiguredKey | SecHaveNetworkKey | SecRequireEncryptedKey

// TCLinkKey is the well-known "ZigBeeAlliance09" trust-centre link key that
// Zigbee 3.0 devices use to join before receiving the real network key.
var TCLinkKey = [16]byte{
	'Z', 'i', 'g', 'B', 'e', 'e', 'A', 'l', 'l', 'i', 'a', 'n', 'c', 'e', '0', '9',
}

// EmberJoinMethod: how a node joins. Coordinators form with MAC association.
const JoinMethodMACAssociation = 0x00

// AllChannelsMask is the 2.4 GHz 802.15.4 channel mask (channels 11-26). The
// network manager may move the network within this set; radioChannel is where
// it actually starts.
const AllChannelsMask uint32 = 0x07FFF800

// NetworkInitNoOptions is EmberNetworkInitBitmask with nothing set: resume a
// stored network as-is.
const NetworkInitNoOptions uint16 = 0x0000

// EncodeInitialSecurityState serialises EmberInitialSecurityState:
// [bitmask u16][preconfiguredKey 16][networkKey 16][keySeqNum u8][tcEui64 8].
// preconfiguredTrustCenterEui64 is left blank: we are the trust centre, so
// there is no other one to pin.
func EncodeInitialSecurityState(bitmask uint16, preconfiguredKey, networkKey [16]byte, keySeq uint8) []byte {
	b := make([]byte, 0, 43)
	b = binary.LittleEndian.AppendUint16(b, bitmask)
	b = append(b, preconfiguredKey[:]...)
	b = append(b, networkKey[:]...)
	b = append(b, keySeq)
	return append(b, make([]byte, 8)...) // blank trust-centre EUI64
}

// EncodeNetworkParameters serialises EmberNetworkParameters for formNetwork.
// The field order matches DecodeNetworkParameters.
func EncodeNetworkParameters(p NetworkParameters) []byte {
	b := make([]byte, 0, netParamsLen)
	b = append(b, p.ExtendedPanID[:]...)
	b = binary.LittleEndian.AppendUint16(b, p.PanID)
	b = append(b, p.RadioTxPower, p.RadioChannel, p.JoinMethod)
	b = binary.LittleEndian.AppendUint16(b, p.NwkManagerID)
	b = append(b, p.NwkUpdateID)
	return binary.LittleEndian.AppendUint32(b, p.Channels)
}

// EncodeAddEndpoint serialises addEndpoint:
// [endpoint][profileId u16][deviceId u16][deviceVersion][inCount][outCount]
// [inClusters u16...][outClusters u16...].
func EncodeAddEndpoint(endpoint uint8, profileID, deviceID uint16, version uint8, in, out []uint16) []byte {
	b := make([]byte, 0, 9+2*(len(in)+len(out)))
	b = append(b, endpoint)
	b = binary.LittleEndian.AppendUint16(b, profileID)
	b = binary.LittleEndian.AppendUint16(b, deviceID)
	b = append(b, version, uint8(len(in)), uint8(len(out)))
	for _, c := range in {
		b = binary.LittleEndian.AppendUint16(b, c)
	}
	for _, c := range out {
		b = binary.LittleEndian.AppendUint16(b, c)
	}
	return b
}

// StatusName renders an EmberStatus for logs.
func StatusName(s uint8) string {
	switch s {
	case StatusSuccess:
		return "SUCCESS"
	case StatusNotJoined:
		return "NOT_JOINED"
	case StatusNetworkUp:
		return "NETWORK_UP"
	case StatusNetworkDown:
		return "NETWORK_DOWN"
	}
	return fmt.Sprintf("%#02x", s)
}

// ---------------------------------------------------------------------------
// M3: letting nodes join.
// ---------------------------------------------------------------------------

// EzspPolicyId / EzspDecisionBitmask for the trust-centre join policy.
const (
	PolicyTrustCenter = 0x00

	DecisionAllowJoins           = 0x0001
	DecisionAllowUnsecuredRejoin = 0x0002
)

// joinPolicy lets new nodes join and lets a node that lost the network key
// rejoin unsecured — the latter is what makes a switch that was power-cycled
// during pairing recoverable instead of bricked out of the network.
const joinPolicy = DecisionAllowJoins | DecisionAllowUnsecuredRejoin

// PermitJoinForever is the duration byte meaning "until told otherwise".
const PermitJoinForever = 0xFF

// EmberDeviceUpdate: why trustCenterJoinHandler fired.
const (
	DeviceSecuredRejoin   = 0x00
	DeviceUnsecuredJoin   = 0x01
	DeviceLeft            = 0x02
	DeviceUnsecuredRejoin = 0x03
)

// TrustCenterJoin is a decoded trustCenterJoinHandler callback.
type TrustCenterJoin struct {
	NodeID   uint16
	IEEE     [8]byte
	Update   uint8 // EmberDeviceUpdate
	Decision uint8 // EmberJoinDecision
	ParentID uint16
}

// IEEEString renders the IEEE address the way configs and logs use it.
func (j TrustCenterJoin) IEEEString() string {
	// The NCP sends EUI64 little-endian; addresses are written big-endian.
	var b [8]byte
	for i := range b {
		b[i] = j.IEEE[7-i]
	}
	return fmt.Sprintf("0x%016X", binary.BigEndian.Uint64(b[:]))
}

// UpdateName renders EmberDeviceUpdate for logs.
func UpdateName(u uint8) string {
	switch u {
	case DeviceSecuredRejoin:
		return "secured-rejoin"
	case DeviceUnsecuredJoin:
		return "join"
	case DeviceLeft:
		return "left"
	case DeviceUnsecuredRejoin:
		return "unsecured-rejoin"
	}
	return fmt.Sprintf("%#02x", u)
}

// DecodeTrustCenterJoin parses a trustCenterJoinHandler payload:
// [nodeId u16][eui64 8][deviceUpdate u8][joinDecision u8][parentNodeId u16].
func DecodeTrustCenterJoin(p []byte) (TrustCenterJoin, error) {
	const want = 2 + 8 + 1 + 1 + 2
	if len(p) < want {
		return TrustCenterJoin{}, fmt.Errorf("ezsp: trustCenterJoin payload too short (%d, want %d)", len(p), want)
	}
	j := TrustCenterJoin{NodeID: binary.LittleEndian.Uint16(p[0:2])}
	copy(j.IEEE[:], p[2:10])
	j.Update = p[10]
	j.Decision = p[11]
	j.ParentID = binary.LittleEndian.Uint16(p[12:14])
	return j, nil
}
