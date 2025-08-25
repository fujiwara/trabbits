// MIT License
// Copyright (c) 2025 FUJIWARA Shunichiro

package trabbits

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"

	"github.com/fujiwara/trabbits/amqp091"
)

var GlobalConfig = sync.Map{}

// Config represents the configuration of the trabbits proxy.
type Config struct {
	Upstreams []UpstreamConfig `yaml:"upstreams" json:"upstreams"`
}

func (c *Config) String() string {
	data, _ := json.MarshalIndent(c, "", "  ")
	return string(data)
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
	slog.Info("Configuration loaded", "config", c)
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

func (c *Config) Validate() error {
	if len(c.Upstreams) == 0 {
		return fmt.Errorf("no upstreams are defined")
	}
	if len(c.Upstreams) > 2 {
		return fmt.Errorf("upstreams must be less or equal than 2 elements")
	}
	for _, u := range c.Upstreams {
		if err := u.Validate(); err != nil {
			return fmt.Errorf("invalid upstream %s: %w", u.Name, err)
		}
	}
	return nil
}

// UpstreamConfig represents the configuration of an upstream server.
type UpstreamConfig struct {
	Name    string         `yaml:"name" json:"name"`
	Host    string         `yaml:"host,omitempty" json:"host,omitempty"`
	Port    int            `yaml:"port,omitempty" json:"port,omitempty"`
	Cluster *ClusterConfig `yaml:"cluster,omitempty" json:"cluster,omitempty"`

	Routing         RoutingConfig    `yaml:"routing,omitempty" json:"routing,omitempty"`
	QueueAttributes *QueueAttributes `yaml:"queue_attributes,omitempty" json:"queue_attributes,omitempty"`
}

func (u *UpstreamConfig) Addresses() []string {
	if u.Cluster != nil {
		var addrs []string
		for _, n := range u.Cluster.Nodes {
			addrs = append(addrs, net.JoinHostPort(n.Host, fmt.Sprintf("%d", n.Port)))
		}
		return addrs
	}
	return []string{net.JoinHostPort(u.Host, fmt.Sprintf("%d", u.Port))}
}

func (u *UpstreamConfig) Validate() error {
	if u.Name == "" {
		return fmt.Errorf("cluster name is required")
	}
	if u.Cluster != nil {
		for _, n := range u.Cluster.Nodes {
			if n.Host == "" {
				return fmt.Errorf("host is required for cluster node")
			}
			if n.Port == 0 {
				return fmt.Errorf("port is required for cluster node")
			}
		}
	} else {
		// single host
		if u.Host == "" {
			return fmt.Errorf("host is required")
		}
		if u.Port == 0 {
			return fmt.Errorf("port is required")
		}
	}
	return nil
}

type ClusterConfig struct {
	Nodes []NodeConfig `yaml:"nodes" json:"nodes"`
}

type NodeConfig struct {
	Host string `yaml:"host" json:"host"`
	Port int    `yaml:"port" json:"port"`
}

type RoutingConfig struct {
	KeyPatterns []string `yaml:"key_patterns,omitempty" json:"key_patterns,omitempty"`
}

type QueueAttributes struct {
	Durable    *bool         `yaml:"durable,omitempty" json:"durable,omitempty"`
	AutoDelete *bool         `yaml:"auto_delete,omitempty" json:"auto_delete,omitempty"`
	Exclusive  *bool         `yaml:"exclusive,omitempty" json:"exclusive,omitempty"`
	Arguments  amqp091.Table `yaml:"arguments,omitempty" json:"arguments,omitempty"`
}
