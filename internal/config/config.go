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
	Zigbee  ZigbeeConfig   `yaml:"zigbee"`
	MQTT    MQTTConfig     `yaml:"mqtt"`
	Devices []DeviceConfig `yaml:"devices"`
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
	}
	return nil
}
