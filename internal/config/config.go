// Package config loads the hub configuration from YAML.
package config

import (
	"fmt"
	"os"

	"github.com/sh5080/home-hub/internal/domain"
	"gopkg.in/yaml.v3"
)

// Config is the top-level hub configuration.
type Config struct {
	HomeKit HomeKitConfig  `yaml:"homekit"`
	Zigbee  ZigbeeConfig   `yaml:"zigbee"`
	MQTT    MQTTConfig     `yaml:"mqtt"`
	Devices []DeviceConfig `yaml:"devices"`
	Rules   []RuleConfig   `yaml:"rules"`
}

// HomeKitConfig configures the HAP bridge exposed to HomeKit controllers.
type HomeKitConfig struct {
	Name    string `yaml:"name"`
	Pin     string `yaml:"pin"`     // 8-digit pairing code
	Port    string `yaml:"port"`    // HAP listen port
	Storage string `yaml:"storage"` // path for pairing/state persistence
}

// ZigbeeConfig configures the Zigbee coordinator.
type ZigbeeConfig struct {
	Port string `yaml:"port"`
}

// MQTTConfig configures the embedded MQTT broker.
type MQTTConfig struct {
	Listen string `yaml:"listen"`
}

// DeviceConfig is a device definition plus optional integration-specific fields.
type DeviceConfig struct {
	domain.Device `yaml:",inline"`

	// Matter-only: which driver backs the device.
	Driver string `yaml:"driver,omitempty"` // "delegated" | "go-matter"
	// Matter delegated-only: action -> HAP virtual trigger switch id.
	Triggers map[string]string `yaml:"triggers,omitempty"`
	// Matter go-matter-only: how to reach the natively-controlled device.
	GoMatter *GoMatterDevice `yaml:"gomatter,omitempty"`
}

// GoMatterDevice locates a Matter device controlled natively by the hub.
type GoMatterDevice struct {
	FabricStore string `yaml:"fabricStore"` // path to fabric credentials
	NodeID      uint64 `yaml:"nodeId"`      // device operational node id
	Address     string `yaml:"address"`     // host:port (default port 5540)
	Endpoint    uint16 `yaml:"endpoint"`    // window-covering endpoint
}

// RuleConfig declares an automation rule. Currently only "mirror" is supported,
// which mirrors the on/off state of Src onto Dst.
type RuleConfig struct {
	Type string `yaml:"type"` // "mirror"
	Src  string `yaml:"src"`
	Dst  string `yaml:"dst"`
}

// Load reads and parses the config file at path.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) validate() error {
	seen := make(map[string]bool)
	for _, d := range c.Devices {
		if d.ID == "" {
			return fmt.Errorf("device with empty id")
		}
		if seen[d.ID] {
			return fmt.Errorf("duplicate device id: %s", d.ID)
		}
		seen[d.ID] = true
		if d.Driver == "go-matter" {
			if d.GoMatter == nil {
				return fmt.Errorf("device %s: driver go-matter requires a gomatter block", d.ID)
			}
			// Address is optional: when empty the device is resolved over mDNS.
			if d.GoMatter.FabricStore == "" || d.GoMatter.NodeID == 0 {
				return fmt.Errorf("device %s: gomatter requires fabricStore and nodeId", d.ID)
			}
		}
	}
	for i, r := range c.Rules {
		if r.Type != "mirror" {
			return fmt.Errorf("rule %d: unknown type %q", i, r.Type)
		}
		if !seen[r.Src] {
			return fmt.Errorf("rule %d: unknown src device %q", i, r.Src)
		}
		if !seen[r.Dst] {
			return fmt.Errorf("rule %d: unknown dst device %q", i, r.Dst)
		}
	}
	return nil
}
