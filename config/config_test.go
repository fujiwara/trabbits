package config_test

import (
	"testing"
	"time"

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

func TestGracefulShutdownDefaults(t *testing.T) {
	cfg := &config.Config{
		Upstreams: []config.Upstream{
			{
				Name:    "test-upstream",
				Address: "localhost:5672",
			},
		},
	}

	cfg.SetDefaults()

	// Check default values
	if cfg.GracefulShutdown.ShutdownTimeout != config.Duration(config.DefaultGracefulShutdownTimeout) {
		t.Errorf("Expected ShutdownTimeout %v, got %v", config.DefaultGracefulShutdownTimeout, cfg.GracefulShutdown.ShutdownTimeout)
	}

	if cfg.GracefulShutdown.ReloadTimeout != config.Duration(config.DefaultGracefulReloadTimeout) {
		t.Errorf("Expected ReloadTimeout %v, got %v", config.DefaultGracefulReloadTimeout, cfg.GracefulShutdown.ReloadTimeout)
	}

	if cfg.GracefulShutdown.RateLimit != config.DefaultDisconnectRateLimit {
		t.Errorf("Expected RateLimit %d, got %d", config.DefaultDisconnectRateLimit, cfg.GracefulShutdown.RateLimit)
	}

	if cfg.GracefulShutdown.BurstSize != config.DefaultDisconnectBurstSize {
		t.Errorf("Expected BurstSize %d, got %d", config.DefaultDisconnectBurstSize, cfg.GracefulShutdown.BurstSize)
	}

	// Test time.Duration conversion
	shutdownDuration := time.Duration(cfg.GracefulShutdown.ShutdownTimeout)
	if shutdownDuration != config.DefaultGracefulShutdownTimeout {
		t.Errorf("Expected shutdown duration %v, got %v", config.DefaultGracefulShutdownTimeout, shutdownDuration)
	}

	t.Logf("✓ ShutdownTimeout: %v", shutdownDuration)
	t.Logf("✓ ReloadTimeout: %v", time.Duration(cfg.GracefulShutdown.ReloadTimeout))
	t.Logf("✓ RateLimit: %d", cfg.GracefulShutdown.RateLimit)
	t.Logf("✓ BurstSize: %d", cfg.GracefulShutdown.BurstSize)
}

func TestGracefulShutdownCustomConfig(t *testing.T) {
	// Test loading custom graceful shutdown configuration
	cfg, err := config.Load(t.Context(), "../testdata/config_with_graceful_shutdown.json")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Check custom values
	expectedShutdownTimeout := 30 * time.Second
	expectedReloadTimeout := 60 * time.Second
	expectedRateLimit := 200
	expectedBurstSize := 20

	if time.Duration(cfg.GracefulShutdown.ShutdownTimeout) != expectedShutdownTimeout {
		t.Errorf("Expected ShutdownTimeout %v, got %v", expectedShutdownTimeout, time.Duration(cfg.GracefulShutdown.ShutdownTimeout))
	}

	if time.Duration(cfg.GracefulShutdown.ReloadTimeout) != expectedReloadTimeout {
		t.Errorf("Expected ReloadTimeout %v, got %v", expectedReloadTimeout, time.Duration(cfg.GracefulShutdown.ReloadTimeout))
	}

	if cfg.GracefulShutdown.RateLimit != expectedRateLimit {
		t.Errorf("Expected RateLimit %d, got %d", expectedRateLimit, cfg.GracefulShutdown.RateLimit)
	}

	if cfg.GracefulShutdown.BurstSize != expectedBurstSize {
		t.Errorf("Expected BurstSize %d, got %d", expectedBurstSize, cfg.GracefulShutdown.BurstSize)
	}

	t.Logf("✓ Custom ShutdownTimeout: %v", time.Duration(cfg.GracefulShutdown.ShutdownTimeout))
	t.Logf("✓ Custom ReloadTimeout: %v", time.Duration(cfg.GracefulShutdown.ReloadTimeout))
	t.Logf("✓ Custom RateLimit: %d", cfg.GracefulShutdown.RateLimit)
	t.Logf("✓ Custom BurstSize: %d", cfg.GracefulShutdown.BurstSize)
}
