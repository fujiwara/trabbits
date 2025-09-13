package config_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fujiwara/trabbits/config"
)

func TestConfigEnvironmentVariableExpansion(t *testing.T) {
	// Set environment variables for test
	t.Setenv("TEST_RABBITMQ_USERNAME", "testuser")
	t.Setenv("TEST_RABBITMQ_PASSWORD", "testpass")
	t.Setenv("TEST_INTERVAL", "45s")

	// Create config file content with environment variables
	configJSON := `{
		"upstreams": [
			{
				"name": "test-cluster",
				"cluster": {
					"nodes": [
						"localhost:5672"
					]
				},
				"health_check": {
					"interval": "${TEST_INTERVAL}",
					"timeout": "5s",
					"unhealthy_threshold": 3,
					"recovery_interval": "60s",
					"username": "${TEST_RABBITMQ_USERNAME}",
					"password": "${TEST_RABBITMQ_PASSWORD}"
				}
			}
		]
	}`

	// Write to temporary file
	tmpfile, err := os.CreateTemp("", "test-config-env-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(configJSON)); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpfile.Close()

	// Load config and verify environment variable expansion
	cfg, err := config.Load(t.Context(), tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify the config was loaded correctly
	if len(cfg.Upstreams) != 1 {
		t.Fatalf("Expected 1 upstream, got %d", len(cfg.Upstreams))
	}

	upstream := cfg.Upstreams[0]
	if upstream.HealthCheck == nil {
		t.Fatal("Expected health check configuration, got nil")
	}

	hc := upstream.HealthCheck

	// Verify environment variables were expanded
	if hc.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", hc.Username)
	}

	if hc.Password.String() != "testpass" {
		t.Errorf("Expected password 'testpass', got '%s'", hc.Password.String())
	}

	expectedInterval := 45 * time.Second
	if hc.Interval.ToDuration() != expectedInterval {
		t.Errorf("Expected interval %v, got %v", expectedInterval, hc.Interval.ToDuration())
	}
}

func TestConfigEnvironmentVariableNotSet(t *testing.T) {
	// Make sure the env var is not set
	t.Setenv("MISSING_ENV_VAR", "")

	configJSON := `{
		"upstreams": [
			{
				"name": "test-cluster",
				"cluster": {
					"nodes": [
						"localhost:5672"
					]
				},
				"health_check": {
					"interval": "30s",
					"timeout": "5s",
					"unhealthy_threshold": 3,
					"recovery_interval": "60s",
					"username": "${MISSING_ENV_VAR}",
					"password": "admin"
				}
			}
		]
	}`

	tmpfile, err := os.CreateTemp("", "test-config-env-missing-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(configJSON)); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpfile.Close()

	// Loading should fail because validation fails for missing username
	_, err = config.Load(t.Context(), tmpfile.Name())
	if err == nil {
		t.Error("Expected config loading to fail for missing environment variable, got nil")
	}

	// Error should be about missing username
	expectedErrorMsg := "username is required for health check"
	if !strings.Contains(err.Error(), expectedErrorMsg) {
		t.Errorf("Expected error to contain '%s', got: %v", expectedErrorMsg, err)
	}
}

func TestConfigWithoutEnvironmentVariables(t *testing.T) {
	t.Parallel()
	// Test that configs without environment variables still work
	configJSON := `{
		"upstreams": [
			{
				"name": "test-cluster",
				"cluster": {
					"nodes": [
						"localhost:5672"
					]
				},
				"health_check": {
					"interval": "30s",
					"timeout": "5s",
					"unhealthy_threshold": 3,
					"recovery_interval": "60s",
					"username": "admin",
					"password": "admin"
				}
			}
		]
	}`

	tmpfile, err := os.CreateTemp("", "test-config-no-env-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(configJSON)); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpfile.Close()

	cfg, err := config.Load(t.Context(), tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify regular values work
	hc := cfg.Upstreams[0].HealthCheck
	if hc.Username != "admin" {
		t.Errorf("Expected username 'admin', got '%s'", hc.Username)
	}
	if hc.Password.String() != "admin" {
		t.Errorf("Expected password 'admin', got '%s'", hc.Password.String())
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("Expected valid config, got error: %v", err)
	}
}

func TestConfigPasswordMasking(t *testing.T) {
	t.Parallel()
	// Test that passwords are masked in String() output
	configJSON := `{
		"upstreams": [
			{
				"name": "test-cluster",
				"cluster": {
					"nodes": [
						"localhost:5672"
					]
				},
				"health_check": {
					"interval": "30s",
					"timeout": "5s",
					"unhealthy_threshold": 3,
					"recovery_interval": "60s",
					"username": "healthcheck",
					"password": "supersecret"
				}
			}
		]
	}`

	tmpfile, err := os.CreateTemp("", "test-config-mask-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(configJSON)); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpfile.Close()

	cfg, err := config.Load(t.Context(), tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Convert to string and check password is masked
	configStr := cfg.String()

	// Should not contain the actual password
	if strings.Contains(configStr, "supersecret") {
		t.Error("Config string contains unmasked password")
	}

	// Should contain the masked password
	if !strings.Contains(configStr, "********") {
		t.Error("Config string should contain masked password")
	}

	// Original config should still have the real password
	if cfg.Upstreams[0].HealthCheck.Password.String() != "supersecret" {
		t.Errorf("Original config password should not be modified, got: %s", cfg.Upstreams[0].HealthCheck.Password.String())
	}
}

func TestConfigJsonnet(t *testing.T) {
	// Set environment variables for test
	t.Setenv("TEST_RABBITMQ_USERNAME", "testuser")
	t.Setenv("TEST_RABBITMQ_PASSWORD", "testpass")
	t.Setenv("TEST_INTERVAL", "45s")

	// Create config file content with environment variables
	configJsonnet := `
local env = std.native('env');
{
  upstreams: [
    {
      name: 'test-cluster',
      cluster: {
        nodes: [
          'localhost:5672',
        ],
      },
      health_check: {
        interval: env('TEST_INTERVAL', ''),
        timeout: '5s',
        unhealthy_threshold: 3,
        recovery_interval: '60s',
        username: env('TEST_RABBITMQ_USERNAME', 'guest'),
        password: env('TEST_RABBITMQ_PASSWORD', 'guest'),
      },
    },
  ],
}`

	// Write to temporary file
	tmpfile, err := os.CreateTemp("", "test-config-env-*.jsonnet")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(configJsonnet)); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpfile.Close()

	// Load config and verify environment variable expansion
	cfg, err := config.Load(t.Context(), tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify the config was loaded correctly
	if len(cfg.Upstreams) != 1 {
		t.Fatalf("Expected 1 upstream, got %d", len(cfg.Upstreams))
	}

	upstream := cfg.Upstreams[0]
	if upstream.HealthCheck == nil {
		t.Fatal("Expected health check configuration, got nil")
	}

	hc := upstream.HealthCheck

	// Verify environment variables were expanded
	if hc.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", hc.Username)
	}

	if hc.Password.String() != "testpass" {
		t.Errorf("Expected password 'testpass', got '%s'", hc.Password.String())
	}

	expectedInterval := 45 * time.Second
	if hc.Interval.ToDuration() != expectedInterval {
		t.Errorf("Expected interval %v, got %v", expectedInterval, hc.Interval.ToDuration())
	}
}
