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
	{
		"one cluster upstream",
		&trabbits.Config{
			Upstreams: []trabbits.UpstreamConfig{
				{
					Cluster: &trabbits.ClusterConfig{
						Name: "test-cluster",
						Nodes: []trabbits.NodeConfig{
							{
								Host: "localhost",
								Port: 5672,
							},
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
					Cluster: &trabbits.ClusterConfig{
						Name: "test-cluster-1",
						Nodes: []trabbits.NodeConfig{
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
				},
				{
					Cluster: &trabbits.ClusterConfig{
						Name: "test-cluster-2",
						Nodes: []trabbits.NodeConfig{
							{
								Host: "localhost",
								Port: 5674,
							},
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
					Host: "localhost",
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
						Nodes: []trabbits.NodeConfig{
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
