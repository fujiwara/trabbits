// MIT License
// Copyright (c) 2025 FUJIWARA Shunichiro

package trabbits

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
)

var GlobalConfig = sync.Map{}

// Config represents the configuration of the trabbits proxy.
type Config struct {
	Upstreams []UpstreamConfig `yaml:"upstreams" json:"upstreams"`
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

const globalConfigKey = "config"

func storeConfig(c *Config) {
	GlobalConfig.Store(globalConfigKey, c)
}

func mustGetConfig() *Config {
	if c, ok := GlobalConfig.Load(globalConfigKey); ok {
		return c.(*Config)
	} else {
		panic("config is not loaded")
	}
}

// UpstreamConfig represents the configuration of an upstream server.
type UpstreamConfig struct {
	Host    string        `yaml:"host" json:"host"`
	Port    int           `yaml:"port" json:"port"`
	Routing RoutingConfig `yaml:"routing" json:"routing"`
}

func (c *Config) Validate() error {
	if len(c.Upstreams) == 0 {
		return fmt.Errorf("no upstreams are defined")
	}
	if len(c.Upstreams) > 2 {
		return fmt.Errorf("upstreams must be less or equal than 2 elements")
	}
	return nil
}

type RoutingConfig struct {
	KeyPatterns []string `yaml:"key_patterns" json:"key_patterns"`
}
