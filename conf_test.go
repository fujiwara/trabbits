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
					Name:    "primary",
					Address: "localhost:5672",
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
					Name:    "primary",
					Address: "localhost:5672",
				},
				{
					Name:    "secondary",
					Address: "localhost:5673",
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
					Name:    "primary",
					Address: "localhost:5672",
				},
				{
					Name:    "secondary",
					Address: "localhost:5673",
				},
				{
					Name:    "tertiary",
					Address: "localhost:5674",
				},
			},
		},
		false,
	},
	{
		"one cluster upstream",
		&trabbits.Config{
			Upstreams: []trabbits.UpstreamConfig{
				{
					Name: "test-cluster",
					Cluster: &trabbits.ClusterConfig{
						Nodes: []string{
							"localhost:5672",
						},
					},
				},
			},
		},
		true,
	},
	{
		"two cluster upstreams",
		&trabbits.Config{
			Upstreams: []trabbits.UpstreamConfig{
				{
					Name: "test-cluster-1",
					Cluster: &trabbits.ClusterConfig{
						Nodes: []string{
							"localhost:5672",
							"localhost:5673",
						},
					},
				},
				{
					Name: "test-cluster-2",
					Cluster: &trabbits.ClusterConfig{
						Nodes: []string{
							"localhost:5674",
						},
					},
				},
			},
		},
		true,
	},
	{
		"invalid empty port",
		&trabbits.Config{
			Upstreams: []trabbits.UpstreamConfig{
				{
					Name:    "primary",
					Address: "localhost",
				},
			},
		},
		false,
	},
	{
		"invalid no cluster name",
		&trabbits.Config{
			Upstreams: []trabbits.UpstreamConfig{
				{
					Cluster: &trabbits.ClusterConfig{
						Nodes: []string{
							"localhost:5672",
							"localhost:5673",
						},
					},
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
			if err != nil {
				t.Log(err)
			}
			if suite.valid && err != nil {
				t.Errorf("unexpected error: %s", err)
			}
			if !suite.valid && err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}
