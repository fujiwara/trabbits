package trabbits

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
)

// Config represents the configuration of the trabbits proxy.
type Config struct {
	Upstreams []UpstreamConfig `yaml:"upstreams" json:"upstreams"`
	Routing   RoutingConfig    `yaml:"routing" json:"routing"`
}

func LoadConfig(f string) (*Config, error) {
	var c Config
	slog.Info("Loading configuration", "file", f)
	data, err := os.ReadFile(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	return &c, nil
}

// UpstreamConfig represents the configuration of an upstream server.
type UpstreamConfig struct {
	Host    string `yaml:"host" json:"host"`
	Port    int    `yaml:"port" json:"port"`
	Default bool   `yaml:"default" json:"default"`
}

func (c *Config) Validate() error {
	if len(c.Upstreams) == 0 {
		return fmt.Errorf("no upstreams are defined")
	}
	if len(c.Upstreams) > 2 {
		return fmt.Errorf("upstreams must be less or equal than 2 elements")
	}
	var defaults int
	for _, u := range c.Upstreams {
		if u.Default {
			defaults++
		}
	}
	if defaults != 1 {
		return fmt.Errorf("default must be equal one section")
	}
	return nil
}

type RoutingConfig struct {
	KeyPatterns []string `yaml:"key_patterns" json:"key_patterns"`
}
