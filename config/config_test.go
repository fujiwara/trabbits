package config_test

import (
	"testing"

	"github.com/fujiwara/trabbits/config"
)

var testConfigSuites = []struct {
	name   string
	config *config.Config
	valid  bool
}{
	{
		"empty",
		&config.Config{},
		false,
	},
	{
		"one upstream",
		&config.Config{
			Upstreams: []config.Upstream{
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
		&config.Config{
			Upstreams: []config.Upstream{
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
		&config.Config{
			Upstreams: []config.Upstream{
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
		&config.Config{
			Upstreams: []config.Upstream{
				{
					Name: "test-cluster",
					Cluster: &config.Cluster{
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
		&config.Config{
			Upstreams: []config.Upstream{
				{
					Name: "test-cluster-1",
					Cluster: &config.Cluster{
						Nodes: []string{
							"localhost:5672",
							"localhost:5673",
						},
					},
				},
				{
					Name: "test-cluster-2",
					Cluster: &config.Cluster{
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
		&config.Config{
			Upstreams: []config.Upstream{
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
		&config.Config{
			Upstreams: []config.Upstream{
				{
					Cluster: &config.Cluster{
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
	t.Parallel()
	for _, suite := range testConfigSuites {
		t.Run(suite.name, func(t *testing.T) {
			t.Parallel()
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
	t.Parallel()
	config1 := &config.Config{
		Upstreams: []config.Upstream{
			{
				Name:    "primary",
				Address: "localhost:5672",
			},
		},
	}

	config2 := &config.Config{
		Upstreams: []config.Upstream{
			{
				Name:    "primary",
				Address: "localhost:5672",
			},
		},
	}

	config3 := &config.Config{
		Upstreams: []config.Upstream{
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
	t.Parallel()
	config1 := &config.Config{
		Upstreams: []config.Upstream{
			{
				Name: "test-cluster",
				Cluster: &config.Cluster{
					Nodes: []string{"localhost:5672"},
				},
				HealthCheck: &config.HealthCheck{
					Username: "admin",
					Password: "secret1",
				},
			},
		},
	}

	config2 := &config.Config{
		Upstreams: []config.Upstream{
			{
				Name: "test-cluster",
				Cluster: &config.Cluster{
					Nodes: []string{"localhost:5672"},
				},
				HealthCheck: &config.HealthCheck{
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
