package trabbits_test

import (
	"testing"

	"github.com/fujiwara/trabbits"
)

var testConfigSuites = []struct {
	name   string
	config *trabbits.Config
	valid  bool
}{
	{
		"empty",
		&trabbits.Config{},
		false,
	},
	{
		"one upstream",
		&trabbits.Config{
			Upstreams: []trabbits.UpstreamConfig{
				{
					Host: "localhost",
					Port: 5672,
				},
			},
		},
		true,
	},
	{
		"two upstream",
		&trabbits.Config{
			Upstreams: []trabbits.UpstreamConfig{
				{
					Host: "localhost",
					Port: 5672,
				},
				{
					Host: "localhost",
					Port: 5673,
				},
			},
		},
		true,
	},
	{
		"three upstreams",
		&trabbits.Config{
			Upstreams: []trabbits.UpstreamConfig{
				{
					Host: "localhost",
					Port: 5672,
				},
				{
					Host: "localhost",
					Port: 5673,
				},
				{
					Host: "localhost",
					Port: 5674,
				},
			},
		},
		false,
	},
}

func TestConfigValidate(t *testing.T) {
	for _, suite := range testConfigSuites {
		t.Run(suite.name, func(t *testing.T) {
			err := suite.config.Validate()
			if suite.valid && err != nil {
				t.Errorf("unexpected error: %s", err)
			}
			if !suite.valid && err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}
