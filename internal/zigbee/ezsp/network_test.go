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
