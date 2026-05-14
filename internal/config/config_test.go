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
		Device: domain.Device{ID: "blind1", Integration: domain.Matter},
		Driver: "go-matter",
	}}}
	if err := c.validate(); err == nil {
		t.Fatal("expected error: go-matter driver without a gomatter block")
	}
}

func TestLoadGoMatterMissingFields(t *testing.T) {
	c := &Config{Devices: []DeviceConfig{{
		Device:   domain.Device{ID: "blind1", Integration: domain.Matter},
		Driver:   "go-matter",
		GoMatter: &GoMatterDevice{FabricStore: "/x", NodeID: 1}, // no address
	}}}
	if err := c.validate(); err == nil {
		t.Fatal("expected error: gomatter missing address")
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
