package config

import (
	"path/filepath"
	"testing"

	"github.com/sh5080/home-hub/internal/domain"
)

func TestLoad(t *testing.T) {
	c, err := Load(filepath.Join("testdata", "devices.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Zigbee.Port != "/dev/ttyUSB0" {
		t.Fatalf("zigbee port = %q", c.Zigbee.Port)
	}
	if len(c.Devices) != 2 {
		t.Fatalf("devices = %d, want 2", len(c.Devices))
	}

	// inline device fields are promoted from the embedded domain.Device
	if c.Devices[0].ID != "s1" || c.Devices[0].Integration != domain.Zigbee {
		t.Fatalf("device 0 = %+v", c.Devices[0])
	}

	// matter-specific fields sit alongside the inlined device
	b := c.Devices[1]
	if b.Integration != domain.Matter || b.Driver != "delegated" || b.Triggers["open"] != "b1_open" {
		t.Fatalf("device 1 = %+v", b)
	}
}

func TestLoadGoMatter(t *testing.T) {
	c, err := Load(filepath.Join("testdata", "gomatter.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	d := c.Devices[0]
	if d.Driver != "go-matter" || d.GoMatter == nil {
		t.Fatalf("device = %+v", d)
	}
	if d.GoMatter.Address != "192.168.1.20:5540" || d.GoMatter.NodeID != 1 || d.GoMatter.Endpoint != 1 {
		t.Fatalf("gomatter = %+v", d.GoMatter)
	}
}

func TestLoadGoMatterMissingBlock(t *testing.T) {
	c := &Config{Devices: []DeviceConfig{{
		Device: domain.Device{ID: "blind1", Integration: domain.Matter, Type: domain.TypeCover},
		Driver: "go-matter",
	}}}
	if err := c.validate(); err == nil {
		t.Fatal("expected error: go-matter driver without a gomatter block")
	}
}

func TestLoadGoMatterMissingFields(t *testing.T) {
	c := &Config{Devices: []DeviceConfig{{
		Device:   domain.Device{ID: "blind1", Integration: domain.Matter, Type: domain.TypeCover},
		Driver:   "go-matter",
		GoMatter: &GoMatterDevice{Address: "192.168.1.20:5540"}, // no fabricStore/nodeId
	}}}
	if err := c.validate(); err == nil {
		t.Fatal("expected error: gomatter missing fabricStore and nodeId")
	}
}

func TestLoadGoMatterAddressOptional(t *testing.T) {
	// No address is valid: the device is resolved over mDNS by node id.
	c := &Config{Devices: []DeviceConfig{{
		Device:   domain.Device{ID: "blind1", Integration: domain.Matter, Type: domain.TypeCover},
		Driver:   "go-matter",
		GoMatter: &GoMatterDevice{FabricStore: "/x/fabric.json", NodeID: 1},
	}}}
	if err := c.validate(); err != nil {
		t.Fatalf("address-less gomatter should validate: %v", err)
	}
}

func TestValidateDeviceRejects(t *testing.T) {
	// A matter device that is not a cover must be rejected (it would be
	// mis-routed as one).
	nonCover := &Config{Devices: []DeviceConfig{{
		Device:   domain.Device{ID: "x", Integration: domain.Matter, Type: domain.TypeLight},
		Driver:   "go-matter",
		GoMatter: &GoMatterDevice{FabricStore: "/x", NodeID: 1},
	}}}
	if err := nonCover.validate(); err == nil {
		t.Fatal("matter non-cover must be rejected")
	}
	// A zigbee device without addr must be rejected.
	noAddr := &Config{Devices: []DeviceConfig{{
		Device: domain.Device{ID: "y", Integration: domain.Zigbee, Type: domain.TypeSwitch},
	}}}
	if err := noAddr.validate(); err == nil {
		t.Fatal("zigbee device without addr must be rejected")
	}
}

func TestLoadDuplicateID(t *testing.T) {
	if _, err := Load(filepath.Join("testdata", "dup.yaml")); err == nil {
		t.Fatal("expected error for duplicate device id")
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join("testdata", "nope.yaml")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

// TestLoadExampleConfig guards the repo's example config: it must always
// parse and validate, since it doubles as the schema documentation.
func TestLoadExampleConfig(t *testing.T) {
	c, err := Load(filepath.Join("..", "..", "configs", "devices.yaml"))
	if err != nil {
		t.Fatalf("example config invalid: %v", err)
	}
	if len(c.Devices) == 0 || len(c.Rules) == 0 {
		t.Fatalf("example config unexpectedly empty: %d devices, %d rules", len(c.Devices), len(c.Rules))
	}
	if c.Zigbee.Storage == "" {
		t.Fatal("example config must demonstrate zigbee storage")
	}
}

// TestLoadPhase1Config guards the Phase 1 (lights-only) starter config from
// docs/ROLLOUT.md so it stays loadable as the schema evolves.
func TestLoadPhase1Config(t *testing.T) {
	c, err := Load(filepath.Join("..", "..", "configs", "phase1-lights.yaml"))
	if err != nil {
		t.Fatalf("phase1 config invalid: %v", err)
	}
	if len(c.Devices) == 0 {
		t.Fatal("phase1 config has no devices")
	}
	for _, d := range c.Devices {
		if d.Type != domain.TypeLight {
			t.Fatalf("phase1 config should be lights only, got %q on %s", d.Type, d.ID)
		}
	}
}

func TestValidateButtonRule(t *testing.T) {
	base := func(r RuleConfig) *Config {
		return &Config{
			Devices: []DeviceConfig{
				{Device: domain.Device{ID: "sw", Integration: domain.Zigbee, Type: domain.TypeSwitch, Addr: "0x01"}},
				{Device: domain.Device{ID: "fan", Integration: domain.MQTT, Type: domain.TypeFan, Addr: "home/fan"}},
			},
			Rules: []RuleConfig{r},
		}
	}
	ok := base(RuleConfig{Type: "button", Src: "sw", Dst: "fan", Press: "single", Action: "toggle"})
	if err := ok.validate(); err != nil {
		t.Fatalf("valid button rule rejected: %v", err)
	}
	if err := base(RuleConfig{Type: "button", Src: "sw", Dst: "fan", Press: "triple", Action: "toggle"}).validate(); err == nil {
		t.Fatal("bad press must be rejected")
	}
	if err := base(RuleConfig{Type: "button", Src: "sw", Dst: "fan", Press: "single", Action: "explode"}).validate(); err == nil {
		t.Fatal("bad action must be rejected")
	}
	if err := base(RuleConfig{Type: "button", Src: "sw", Dst: "fan", Press: "single", Action: "position", Value: 150}).validate(); err == nil {
		t.Fatal("out-of-range position must be rejected")
	}
}

func TestValidateThresholdRule(t *testing.T) {
	above, below := 65.0, 55.0
	c := &Config{
		Devices: []DeviceConfig{
			{Device: domain.Device{ID: "hum", Integration: domain.MQTT, Type: domain.TypeHumidity, Addr: "home/hum"}},
			{Device: domain.Device{ID: "dehum", Integration: domain.MQTT, Type: domain.TypeSwitch, Addr: "home/dehum"}},
		},
		Rules: []RuleConfig{{Type: "threshold", Src: "hum", Dst: "dehum", Above: &above, Below: &below}},
	}
	if err := c.validate(); err != nil {
		t.Fatalf("valid threshold rule rejected: %v", err)
	}
	bad := 50.0
	c.Rules[0].Above = &bad // above <= below: no hysteresis band
	if err := c.validate(); err == nil {
		t.Fatal("inverted threshold band must be rejected")
	}
}

func TestValidateDelegatedNeedsTriggers(t *testing.T) {
	c := &Config{Devices: []DeviceConfig{{
		Device: domain.Device{ID: "b1", Integration: domain.Matter, Type: domain.TypeCover},
		Driver: "delegated",
	}}}
	if err := c.validate(); err == nil {
		t.Fatal("delegated matter device without triggers must be rejected")
	}
}

func TestLoadRules(t *testing.T) {
	c, err := Load(filepath.Join("testdata", "devices.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Rules) != 1 {
		t.Fatalf("rules = %d, want 1", len(c.Rules))
	}
	if r := c.Rules[0]; r.Type != "mirror" || r.Src != "s1" || r.Dst != "b1" {
		t.Fatalf("rule 0 = %+v", r)
	}
}
