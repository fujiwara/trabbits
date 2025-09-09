// MIT License
// Copyright (c) 2025 FUJIWARA Shunichiro

package trabbits

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	armed "github.com/fujiwara/jsonnet-armed"
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

	buf := new(bytes.Buffer)
	if strings.HasSuffix(f, ".jsonnet") {
		cli := &armed.CLI{
			Filename: f,
		}
		cli.SetWriter(buf)
		if err := cli.Run(context.TODO()); err != nil {
			return nil, fmt.Errorf("failed to run load config: %w", err)
		}
	} else {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
		// Expand environment variables in the configuration
		expandedData := os.ExpandEnv(string(data))
		buf.WriteString(expandedData)
	}

	if err := json.Unmarshal(buf.Bytes(), &c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	slog.Info("Configuration loaded", "config", c.String())
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
	Name        string             `yaml:"name" json:"name"`
	Address     string             `yaml:"address,omitempty" json:"address,omitempty"`
	Cluster     *ClusterConfig     `yaml:"cluster,omitempty" json:"cluster,omitempty"`
	Timeout     Duration           `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	HealthCheck *HealthCheckConfig `yaml:"health_check,omitempty" json:"health_check,omitempty"`

	Routing         RoutingConfig    `yaml:"routing,omitempty" json:"routing,omitempty"`
	QueueAttributes *QueueAttributes `yaml:"queue_attributes,omitempty" json:"queue_attributes,omitempty"`
}

func (u *UpstreamConfig) Addresses() []string {
	if u.Cluster != nil {
		return u.Cluster.Nodes
	}
	return []string{u.Address}
}

func (u *UpstreamConfig) Validate() error {
	if u.Name == "" {
		return fmt.Errorf("cluster name is required")
	}
	if u.Cluster != nil {
		for _, addr := range u.Cluster.Nodes {
			if addr == "" {
				return fmt.Errorf("address is required for cluster node")
			}
			// Validate address format
			if _, _, err := net.SplitHostPort(addr); err != nil {
				return fmt.Errorf("invalid address format for cluster node: %w", err)
			}
		}
		// Validate health check credentials if health check is defined
		if u.HealthCheck != nil {
			if u.HealthCheck.Username == "" {
				return fmt.Errorf("username is required for health check")
			}
			if u.HealthCheck.Password == "" {
				return fmt.Errorf("password is required for health check")
			}
		}
	} else {
		// single host
		if u.Address == "" {
			return fmt.Errorf("address is required")
		}
		// Validate address format
		if _, _, err := net.SplitHostPort(u.Address); err != nil {
			return fmt.Errorf("invalid address format: %w", err)
		}
	}
	return nil
}

type ClusterConfig struct {
	Nodes []string `yaml:"nodes" json:"nodes"`
}

// Password is a custom type that masks the value during JSON marshaling
type Password string

// UnmarshalJSON implements json.Unmarshaler interface
func (p *Password) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*p = Password(s)
	return nil
}

// MarshalJSON implements json.Marshaler interface to mask password
func (p Password) MarshalJSON() ([]byte, error) {
	if p == "" {
		return json.Marshal("")
	}
	return json.Marshal("********")
}

// String returns the actual password value (for internal use)
func (p Password) String() string {
	return string(p)
}

// Duration is a custom type that can unmarshal duration strings from JSON
type Duration time.Duration

// UnmarshalJSON implements json.Unmarshaler interface
func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}

	*d = Duration(dur)
	return nil
}

// MarshalJSON implements json.Marshaler interface
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// String returns string representation
func (d Duration) String() string {
	return time.Duration(d).String()
}

// ToDuration converts to time.Duration
func (d Duration) ToDuration() time.Duration {
	return time.Duration(d)
}

// HealthCheckConfig represents health check configuration for cluster upstreams
type HealthCheckConfig struct {
	Interval           Duration `yaml:"interval,omitempty" json:"interval,omitempty"`
	Timeout            Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	UnhealthyThreshold int      `yaml:"unhealthy_threshold,omitempty" json:"unhealthy_threshold,omitempty"`
	RecoveryInterval   Duration `yaml:"recovery_interval,omitempty" json:"recovery_interval,omitempty"`
	Username           string   `yaml:"username,omitempty" json:"username,omitempty"`
	Password           Password `yaml:"password,omitempty" json:"password,omitempty"`
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
