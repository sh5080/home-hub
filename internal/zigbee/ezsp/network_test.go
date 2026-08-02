package ezsp

import "testing"

func TestDecodeNetworkParametersJoined(t *testing.T) {
	p := []byte{
		StatusSuccess,       // status
		NodeTypeCoordinator, // node type
		// EmberNetworkParameters
		0xDD, 0xDD, 0xDD, 0xDD, 0xDD, 0xDD, 0xDD, 0xDD, // extended pan id
		0x34, 0x12, // pan id 0x1234, little-endian
		0x08,       // tx power 8 dBm
		0x0F,       // channel 15
		0x00,       // join method
		0x00, 0x00, // network manager id
		0x00,                   // network update id
		0x00, 0x80, 0x00, 0x00, // channel mask
	}
	s, err := DecodeNetworkParameters(p)
	if err != nil {
		t.Fatalf("DecodeNetworkParameters: %v", err)
	}
	if !s.Joined() {
		t.Fatal("Joined = false, want true on SUCCESS")
	}
	if s.Params.PanID != 0x1234 {
		t.Fatalf("PanID = %#04x, want 0x1234", s.Params.PanID)
	}
	if s.Params.RadioChannel != 15 || s.Params.RadioTxPower != 8 {
		t.Fatalf("channel=%d txPower=%d", s.Params.RadioChannel, s.Params.RadioTxPower)
	}
	if s.Params.Channels != 0x00008000 {
		t.Fatalf("Channels = %#08x", s.Params.Channels)
	}
	if NodeTypeName(s.NodeType) != "coordinator" {
		t.Fatalf("NodeTypeName = %q", NodeTypeName(s.NodeType))
	}
}

// A factory-fresh dongle answers NOT_JOINED. That is the M2 trigger, not an
// error, so it must decode cleanly rather than being rejected.
func TestDecodeNetworkParametersNotJoined(t *testing.T) {
	p := make([]byte, 2+netParamsLen)
	p[0] = StatusNotJoined
	p[1] = NodeTypeUnknown
	s, err := DecodeNetworkParameters(p)
	if err != nil {
		t.Fatalf("DecodeNetworkParameters: %v", err)
	}
	if s.Joined() {
		t.Fatal("Joined = true, want false on NOT_JOINED")
	}
}

func TestDecodeNetworkParametersShort(t *testing.T) {
	if _, err := DecodeNetworkParameters(make([]byte, 2+netParamsLen-1)); err == nil {
		t.Fatal("accepted a short getNetworkParameters payload")
	}
}

// Encode and Decode must agree on field order. A silent disagreement here would
// form a network on the wrong channel or PAN id, which only shows up as devices
// mysteriously failing to pair.
func TestNetworkParametersRoundTrip(t *testing.T) {
	want := NetworkParameters{
		ExtendedPanID: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
		PanID:         0xABCD,
		RadioTxPower:  5,
		RadioChannel:  25,
		JoinMethod:    JoinMethodMACAssociation,
		NwkManagerID:  0x0000,
		NwkUpdateID:   0,
		Channels:      AllChannelsMask,
	}
	encoded := EncodeNetworkParameters(want)
	if len(encoded) != netParamsLen {
		t.Fatalf("encoded length = %d, want %d", len(encoded), netParamsLen)
	}
	// Prepend the status/nodeType the NCP puts in front of a read.
	got, err := DecodeNetworkParameters(append([]byte{StatusSuccess, NodeTypeCoordinator}, encoded...))
	if err != nil {
		t.Fatalf("DecodeNetworkParameters: %v", err)
	}
	if got.Params != want {
		t.Fatalf("round trip:\n got %+v\nwant %+v", got.Params, want)
	}
}

func TestEncodeInitialSecurityState(t *testing.T) {
	var nwk [16]byte
	for i := range nwk {
		nwk[i] = byte(i)
	}
	b := EncodeInitialSecurityState(formSecurityBitmask, TCLinkKey, nwk, 0)
	if len(b) != 43 {
		t.Fatalf("length = %d, want 43 (2+16+16+1+8)", len(b))
	}
	if b[0] != byte(formSecurityBitmask&0xFF) || b[1] != byte(formSecurityBitmask>>8) {
		t.Fatalf("bitmask bytes = %#02x %#02x", b[0], b[1])
	}
	if string(b[2:18]) != "ZigBeeAlliance09" {
		t.Fatalf("preconfigured key = %q, want the well-known TC link key", b[2:18])
	}
	if !bytesEqual(b[18:34], nwk[:]) {
		t.Fatalf("network key misplaced: % X", b[18:34])
	}
	if b[34] != 0 {
		t.Fatalf("key sequence = %d, want 0", b[34])
	}
	for i, v := range b[35:43] {
		if v != 0 {
			t.Fatalf("trust-centre EUI64 byte %d = %#02x, want blank", i, v)
		}
	}
}

// The formation profile must keep the network key off the air in the clear and
// use the global trust-centre link key, or Zigbee 3.0 devices refuse to join.
func TestFormSecurityBitmaskFlags(t *testing.T) {
	for _, f := range []struct {
		name string
		bit  int
	}{
		{"hashed trust-centre link key", SecTrustCenterHashedLinkKey},
		{"have preconfigured key", SecHavePreconfiguredKey},
		{"have network key", SecHaveNetworkKey},
		{"require encrypted key", SecRequireEncryptedKey},
	} {
		if formSecurityBitmask&f.bit != f.bit {
			t.Errorf("formSecurityBitmask missing %s (%#04x)", f.name, f.bit)
		}
	}
}

func TestEncodeAddEndpoint(t *testing.T) {
	b := EncodeAddEndpoint(1, profileHA, deviceIDTool, 0, []uint16{0x0000}, []uint16{0x0006, 0x0102})
	want := []byte{
		0x01,       // endpoint
		0x04, 0x01, // profile 0x0104 LE
		0x05, 0x00, // device 0x0005 LE
		0x00,       // device version
		0x01,       // input count
		0x02,       // output count
		0x00, 0x00, // input: Basic
		0x06, 0x00, // output: On/Off
		0x02, 0x01, // output: Window Covering 0x0102 LE
	}
	if !bytesEqual(b, want) {
		t.Fatalf("EncodeAddEndpoint = % X, want % X", b, want)
	}
}

func TestRandomPanIDAvoidsReserved(t *testing.T) {
	for i := 0; i < 200; i++ {
		id, err := randomPanID()
		if err != nil {
			t.Fatalf("randomPanID: %v", err)
		}
		if id == 0x0000 || id == 0xFFFF {
			t.Fatalf("drew reserved pan id %#04x", id)
		}
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
