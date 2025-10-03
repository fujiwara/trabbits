// MIT License
// Copyright (c) 2025 FUJIWARA Shunichiro

package config

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"time"

	armed "github.com/fujiwara/jsonnet-armed"
	"github.com/fujiwara/trabbits/amqp091"
)

// Default values for graceful shutdown
const (
	DefaultGracefulShutdownTimeout = 10 * time.Second
	DefaultGracefulReloadTimeout   = 30 * time.Second
	DefaultDisconnectRateLimit     = 100 // connections per second
	DefaultDisconnectBurstSize     = 10
)

// Config represents the configuration of the trabbits proxy.
type Config struct {
	Upstreams              []Upstream       `yaml:"upstreams" json:"upstreams"`
	ReadTimeout            Duration         `yaml:"read_timeout,omitempty" json:"read_timeout,omitempty"`
	ConnectionCloseTimeout Duration         `yaml:"connection_close_timeout,omitempty" json:"connection_close_timeout,omitempty"`
	GracefulShutdown       GracefulShutdown `yaml:"graceful_shutdown,omitempty" json:"graceful_shutdown,omitempty"`
}

// GracefulShutdown configures graceful shutdown behavior
type GracefulShutdown struct {
	ShutdownTimeout Duration `yaml:"shutdown_timeout,omitempty" json:"shutdown_timeout,omitempty"` // Max time to wait for shutdown (SIGTERM)
	ReloadTimeout   Duration `yaml:"reload_timeout,omitempty" json:"reload_timeout,omitempty"`     // Max time to wait for config reload (SIGHUP)
	RateLimit       int      `yaml:"rate_limit,omitempty" json:"rate_limit,omitempty"`             // Disconnections per second
	BurstSize       int      `yaml:"burst_size,omitempty" json:"burst_size,omitempty"`             // Initial burst allowance
}

// Hash calculates SHA256 hash of the config using gob encoding
func (c *Config) Hash() string {
	// Use gob encoding to serialize config directly to hash writer
	hasher := sha256.New()
	encoder := gob.NewEncoder(hasher)

	if err := encoder.Encode(c); err != nil {
		slog.Error("Failed to encode config with gob for hashing", "error", err)
		return ""
	}

	return hex.EncodeToString(hasher.Sum(nil))
}

func (c *Config) String() string {
	data, _ := json.MarshalIndent(c, "", "  ")
	return string(data)
}

func Load(ctx context.Context, f string) (*Config, error) {
	var c Config
	slog.Info("Loading configuration", "file", f)

	buf := new(bytes.Buffer)
	if strings.HasSuffix(f, ".jsonnet") {
		cli := &armed.CLI{
			Filename: f,
		}
		cli.SetWriter(buf)
		if err := cli.Run(ctx); err != nil {
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
	c.SetDefaults()
	return &c, nil
}

// SetDefaults sets default values for config fields if not specified
func (c *Config) SetDefaults() {
	// Set default timeout values if not specified
	if c.ReadTimeout == 0 {
		c.ReadTimeout = Duration(5 * time.Second) // DefaultReadTimeout
	}
	if c.ConnectionCloseTimeout == 0 {
		c.ConnectionCloseTimeout = Duration(1 * time.Second) // DefaultConnectionCloseTimeout
	}

	// Set default graceful shutdown values if not specified
	if c.GracefulShutdown.ShutdownTimeout == 0 {
		c.GracefulShutdown.ShutdownTimeout = Duration(DefaultGracefulShutdownTimeout)
	}
	if c.GracefulShutdown.ReloadTimeout == 0 {
		c.GracefulShutdown.ReloadTimeout = Duration(DefaultGracefulReloadTimeout)
	}
	if c.GracefulShutdown.RateLimit == 0 {
		c.GracefulShutdown.RateLimit = DefaultDisconnectRateLimit
	}
	if c.GracefulShutdown.BurstSize == 0 {
		c.GracefulShutdown.BurstSize = DefaultDisconnectBurstSize
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

// Upstream represents the configuration of an upstream server.
type Upstream struct {
	Name        string       `yaml:"name" json:"name"`
	Address     string       `yaml:"address,omitempty" json:"address,omitempty"`
	Cluster     *Cluster     `yaml:"cluster,omitempty" json:"cluster,omitempty"`
	Timeout     Duration     `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	HealthCheck *HealthCheck `yaml:"health_check,omitempty" json:"health_check,omitempty"`

	Routing         Routing          `yaml:"routing,omitempty" json:"routing,omitempty"`
	QueueAttributes *QueueAttributes `yaml:"queue_attributes,omitempty" json:"queue_attributes,omitempty"`
}

func (u *Upstream) Addresses() []string {
	if u.Cluster != nil {
		return u.Cluster.Nodes
	}
	return []string{u.Address}
}

func (u *Upstream) Validate() error {
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

type Cluster struct {
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

// HealthCheck represents health check configuration for cluster upstreams
type HealthCheck struct {
	Interval           Duration `yaml:"interval,omitempty" json:"interval,omitempty"`
	Timeout            Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	UnhealthyThreshold int      `yaml:"unhealthy_threshold,omitempty" json:"unhealthy_threshold,omitempty"`
	RecoveryInterval   Duration `yaml:"recovery_interval,omitempty" json:"recovery_interval,omitempty"`
	Username           string   `yaml:"username,omitempty" json:"username,omitempty"`
	Password           Password `yaml:"password,omitempty" json:"password,omitempty"`
}

type Routing struct {
	KeyPatterns []string `yaml:"key_patterns,omitempty" json:"key_patterns,omitempty"`
}

type QueueAttributes struct {
	Durable           *bool         `yaml:"durable,omitempty" json:"durable,omitempty"`
	AutoDelete        *bool         `yaml:"auto_delete,omitempty" json:"auto_delete,omitempty"`
	Exclusive         *bool         `yaml:"exclusive,omitempty" json:"exclusive,omitempty"`
	Arguments         amqp091.Table `yaml:"arguments,omitempty" json:"arguments,omitempty"`
	TryPassive        bool          `yaml:"try_passive,omitempty" json:"try_passive,omitempty"`
	EmulateAutoDelete bool          `yaml:"emulate_auto_delete,omitempty" json:"emulate_auto_delete,omitempty"`
}
