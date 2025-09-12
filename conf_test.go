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

func TestConfigHash(t *testing.T) {
	config1 := &trabbits.Config{
		Upstreams: []trabbits.UpstreamConfig{
			{
				Name:    "primary",
				Address: "localhost:5672",
			},
		},
	}

	config2 := &trabbits.Config{
		Upstreams: []trabbits.UpstreamConfig{
			{
				Name:    "primary",
				Address: "localhost:5672",
			},
		},
	}

	config3 := &trabbits.Config{
		Upstreams: []trabbits.UpstreamConfig{
			{
				Name:    "primary",
				Address: "localhost:5673", // different port
			},
		},
	}

	hash1 := config1.Hash()
	hash2 := config2.Hash()
	hash3 := config3.Hash()

	if hash1 == "" {
		t.Error("hash1 should not be empty")
	}
	if hash1 != hash2 {
		t.Errorf("identical configs should have same hash: %s != %s", hash1, hash2)
	}
	if hash1 == hash3 {
		t.Errorf("different configs should have different hash: %s == %s", hash1, hash3)
	}

	t.Logf("hash1: %s", hash1)
	t.Logf("hash2: %s", hash2)
	t.Logf("hash3: %s", hash3)
}

func TestConfigHashWithPassword(t *testing.T) {
	config1 := &trabbits.Config{
		Upstreams: []trabbits.UpstreamConfig{
			{
				Name: "test-cluster",
				Cluster: &trabbits.ClusterConfig{
					Nodes: []string{"localhost:5672"},
				},
				HealthCheck: &trabbits.HealthCheckConfig{
					Username: "admin",
					Password: "secret1",
				},
			},
		},
	}

	config2 := &trabbits.Config{
		Upstreams: []trabbits.UpstreamConfig{
			{
				Name: "test-cluster",
				Cluster: &trabbits.ClusterConfig{
					Nodes: []string{"localhost:5672"},
				},
				HealthCheck: &trabbits.HealthCheckConfig{
					Username: "admin",
					Password: "secret2", // different password
				},
			},
		},
	}

	hash1 := config1.Hash()
	hash2 := config2.Hash()

	if hash1 == hash2 {
		t.Errorf("configs with different passwords should have different hashes: %s == %s", hash1, hash2)
	}

	t.Logf("config with password1 hash: %s", hash1)
	t.Logf("config with password2 hash: %s", hash2)
}
