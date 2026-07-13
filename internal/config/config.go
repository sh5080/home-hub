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
	// Storage directory for network + pairing persistence. Without it the hub
	// forms a NEW network on every restart and all devices must re-pair.
	Storage string `yaml:"storage"`
	// PermitJoin opens the network for pairing. Enable only while pairing.
	PermitJoin bool `yaml:"permitJoin"`
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

// RuleConfig declares an automation rule.
//
//   - mirror:    mirror the on/off state of Src onto Dst.
//   - button:    when Src emits a button event with Press, run Action on Dst
//     (toggle | on | off | open | close | position, with Value for position).
//   - cycle:     each Press from Src steps Dst through States (levels 0..100,
//     0=off) — e.g. off→50→100→off. Power (optional) resets the cycle when it
//     turns off (L1 relay / all-off).
//   - threshold: hysteresis on a sensor value — Dst turns on at/above Above
//     and off at/below Below (e.g. humidity → dehumidifier).
type RuleConfig struct {
	Type string `yaml:"type"` // "mirror" | "button" | "cycle" | "threshold"
	Src  string `yaml:"src"`
	Dst  string `yaml:"dst"`

	// button rules
	Press  string `yaml:"press,omitempty"`  // single | double | hold
	Action string `yaml:"action,omitempty"` // toggle | on | off | open | close | position
	Value  int    `yaml:"value,omitempty"`  // position percent for action=position

	// cycle rules
	States []int  `yaml:"states,omitempty"` // levels to step through, 0=off
	Power  string `yaml:"power,omitempty"`  // optional device whose off resets the cycle

	// threshold rules
	Above *float64 `yaml:"above,omitempty"`
	Below *float64 `yaml:"below,omitempty"`
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
	byID := make(map[string]domain.Device, len(c.Devices))
	for _, d := range c.Devices {
		if d.ID == "" {
			return fmt.Errorf("device with empty id")
		}
		if _, dup := byID[d.ID]; dup {
			return fmt.Errorf("duplicate device id: %s", d.ID)
		}
		byID[d.ID] = d.Device
		if err := validateDevice(d); err != nil {
			return err
		}
	}
	for i, r := range c.Rules {
		if err := validateRule(i, r, byID); err != nil {
			return err
		}
	}
	return nil
}

// validTypes is the set of device types the hub knows how to expose/route.
var validTypes = map[domain.DeviceType]bool{
	domain.TypeSwitch: true, domain.TypeLight: true, domain.TypeFan: true,
	domain.TypeCover: true, domain.TypeSensor: true, domain.TypeHumidity: true,
}

func validateDevice(d DeviceConfig) error {
	if !validTypes[d.Type] {
		return fmt.Errorf("device %s: unknown type %q", d.ID, d.Type)
	}
	switch d.Integration {
	case domain.Zigbee, domain.MQTT:
		if d.Addr == "" {
			return fmt.Errorf("device %s: %s device requires addr", d.ID, d.Integration)
		}
	case domain.Matter:
		// Matter devices are routed as covers (open/close/position); any other
		// type would be silently mishandled by the command router.
		if d.Type != domain.TypeCover {
			return fmt.Errorf("device %s: matter devices must be type cover, got %q", d.ID, d.Type)
		}
		return validateMatterDevice(d)
	default:
		return fmt.Errorf("device %s: unknown integration %q", d.ID, d.Integration)
	}
	return nil
}

func validateMatterDevice(d DeviceConfig) error {
	switch d.Driver {
	case "go-matter":
		if d.GoMatter == nil {
			return fmt.Errorf("device %s: driver go-matter requires a gomatter block", d.ID)
		}
		// Address is optional: when empty the device is resolved over mDNS.
		if d.GoMatter.FabricStore == "" || d.GoMatter.NodeID == 0 {
			return fmt.Errorf("device %s: gomatter requires fabricStore and nodeId", d.ID)
		}
	case "delegated", "":
		// Delegation drives the device through HomeKit trigger automations, so
		// both trigger switches must be named.
		if d.Triggers["open"] == "" || d.Triggers["close"] == "" {
			return fmt.Errorf("device %s: delegated matter device requires triggers.open and triggers.close", d.ID)
		}
	default:
		return fmt.Errorf("device %s: unknown matter driver %q", d.ID, d.Driver)
	}
	return nil
}

func validateRule(i int, r RuleConfig, byID map[string]domain.Device) error {
	src, ok := byID[r.Src]
	if !ok {
		return fmt.Errorf("rule %d: unknown src device %q", i, r.Src)
	}
	if _, ok := byID[r.Dst]; !ok {
		return fmt.Errorf("rule %d: unknown dst device %q", i, r.Dst)
	}
	switch r.Type {
	case "mirror":
	case "button":
		if src.Integration != domain.Zigbee {
			return fmt.Errorf("rule %d: button src %q must be a zigbee device (only those emit button events)", i, r.Src)
		}
		switch r.Press {
		case "single", "double", "hold":
		default:
			return fmt.Errorf("rule %d: button press must be single|double|hold, got %q", i, r.Press)
		}
		switch r.Action {
		case "toggle", "on", "off", "open", "close":
		case "position":
			if r.Value < 0 || r.Value > 100 {
				return fmt.Errorf("rule %d: position value %d out of range [0,100]", i, r.Value)
			}
		default:
			return fmt.Errorf("rule %d: unknown button action %q", i, r.Action)
		}
	case "cycle":
		if src.Integration != domain.Zigbee {
			return fmt.Errorf("rule %d: cycle src %q must be a zigbee device (only those emit button events)", i, r.Src)
		}
		switch r.Press {
		case "single", "double", "hold":
		default:
			return fmt.Errorf("rule %d: cycle press must be single|double|hold, got %q", i, r.Press)
		}
		if len(r.States) < 2 {
			return fmt.Errorf("rule %d: cycle needs at least 2 states", i)
		}
		for _, s := range r.States {
			if s < 0 || s > 100 {
				return fmt.Errorf("rule %d: cycle state %d out of range [0,100]", i, s)
			}
		}
		if r.Power != "" {
			if _, ok := byID[r.Power]; !ok {
				return fmt.Errorf("rule %d: unknown cycle power device %q", i, r.Power)
			}
		}
	case "threshold":
		if src.Type != domain.TypeSensor && src.Type != domain.TypeHumidity {
			return fmt.Errorf("rule %d: threshold src %q must be a sensor/humidity device", i, r.Src)
		}
		if r.Above == nil || r.Below == nil {
			return fmt.Errorf("rule %d: threshold requires above and below", i)
		}
		if *r.Above <= *r.Below {
			return fmt.Errorf("rule %d: above (%g) must exceed below (%g) for hysteresis", i, *r.Above, *r.Below)
		}
	default:
		return fmt.Errorf("rule %d: unknown type %q", i, r.Type)
	}
	return nil
}
