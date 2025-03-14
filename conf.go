package trabbits

import "fmt"

// Config represents the configuration of the trabbits proxy.
type Config struct {
	Upstreams []UpstreamConfig `yaml:"upstreams" json:"upstreams"`
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
