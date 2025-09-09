package trabbits_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/fujiwara/trabbits"
)

func TestConfigJSONParsingWithHealthCheck(t *testing.T) {
	// Test JSON config with health check duration strings
	configJSON := `{
		"upstreams": [
			{
				"name": "primary",
				"address": "localhost:5672"
			},
			{
				"name": "secondary",
				"cluster": {
					"nodes": [
						"localhost:5673",
						"localhost:5674",
						"localhost:5675"
					]
				},
				"timeout": "10s",
				"health_check": {
					"interval": "30s",
					"timeout": "5s",
					"unhealthy_threshold": 3,
					"recovery_interval": "60s",
					"username": "admin",
					"password": "admin"
				},
				"routing": {
					"key_patterns": ["test.queue.*"]
				}
			}
		]
	}`

	var cfg trabbits.Config
	err := json.Unmarshal([]byte(configJSON), &cfg)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON config: %v", err)
	}

	// Validate the config
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Config validation failed: %v", err)
	}

	// Check upstream count
	if len(cfg.Upstreams) != 2 {
		t.Errorf("Expected 2 upstreams, got %d", len(cfg.Upstreams))
	}

	// Check first upstream (single host)
	primary := cfg.Upstreams[0]
	if primary.Name != "primary" {
		t.Errorf("Expected primary name 'primary', got '%s'", primary.Name)
	}
	if primary.Address != "localhost:5672" {
		t.Errorf("Expected primary address 'localhost:5672', got '%s'", primary.Address)
	}

	// Check second upstream (cluster with health check)
	secondary := cfg.Upstreams[1]
	if secondary.Name != "secondary" {
		t.Errorf("Expected secondary name 'secondary', got '%s'", secondary.Name)
	}

	// Check cluster configuration
	if secondary.Cluster == nil {
		t.Fatal("Expected cluster configuration, got nil")
	}
	if len(secondary.Cluster.Nodes) != 3 {
		t.Errorf("Expected 3 cluster nodes, got %d", len(secondary.Cluster.Nodes))
	}

	// Check timeout parsing
	expectedTimeout := 10 * time.Second
	if secondary.Timeout.ToDuration() != expectedTimeout {
		t.Errorf("Expected timeout %v, got %v", expectedTimeout, secondary.Timeout.ToDuration())
	}

	// Check health check configuration
	if secondary.HealthCheck == nil {
		t.Fatal("Expected health check configuration, got nil")
	}

	hc := secondary.HealthCheck

	expectedInterval := 30 * time.Second
	if hc.Interval.ToDuration() != expectedInterval {
		t.Errorf("Expected interval %v, got %v", expectedInterval, hc.Interval.ToDuration())
	}

	expectedHCTimeout := 5 * time.Second
	if hc.Timeout.ToDuration() != expectedHCTimeout {
		t.Errorf("Expected health check timeout %v, got %v", expectedHCTimeout, hc.Timeout.ToDuration())
	}

	if hc.UnhealthyThreshold != 3 {
		t.Errorf("Expected unhealthy threshold 3, got %d", hc.UnhealthyThreshold)
	}

	expectedRecovery := 60 * time.Second
	if hc.RecoveryInterval.ToDuration() != expectedRecovery {
		t.Errorf("Expected recovery interval %v, got %v", expectedRecovery, hc.RecoveryInterval.ToDuration())
	}

	// Check routing configuration
	if len(secondary.Routing.KeyPatterns) != 1 {
		t.Errorf("Expected 1 routing pattern, got %d", len(secondary.Routing.KeyPatterns))
	}
	if secondary.Routing.KeyPatterns[0] != "test.queue.*" {
		t.Errorf("Expected routing pattern 'test.queue.*', got '%s'", secondary.Routing.KeyPatterns[0])
	}
}

func TestConfigFileLoading(t *testing.T) {
	// Test loading the actual testdata config file
	cfg, err := trabbits.LoadConfig(t.Context(), "testdata/config.json")
	if err != nil {
		t.Fatalf("Failed to load config file: %v", err)
	}

	// Validate the loaded config
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Loaded config validation failed: %v", err)
	}

	// Check that health check settings are properly loaded
	var foundCluster bool
	for _, upstream := range cfg.Upstreams {
		if upstream.Cluster != nil && upstream.HealthCheck != nil {
			foundCluster = true
			hc := upstream.HealthCheck

			// Check that duration fields are properly parsed
			if hc.Interval.ToDuration() != 3*time.Second {
				t.Errorf("Expected interval 3s, got %v", hc.Interval.ToDuration())
			}
			if hc.Timeout.ToDuration() != 2*time.Second {
				t.Errorf("Expected timeout 2s, got %v", hc.Timeout.ToDuration())
			}
			if hc.RecoveryInterval.ToDuration() != 10*time.Second {
				t.Errorf("Expected recovery interval 10s, got %v", hc.RecoveryInterval.ToDuration())
			}
		}
	}

	if !foundCluster {
		t.Error("Expected to find at least one cluster upstream with health check")
	}
}

func TestDurationJSONMarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		duration string
		expected time.Duration
	}{
		{"seconds", "30s", 30 * time.Second},
		{"minutes", "5m", 5 * time.Minute},
		{"milliseconds", "500ms", 500 * time.Millisecond},
		{"mixed", "1m30s", 90 * time.Second},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Test unmarshaling from JSON string
			jsonStr := `"` + test.duration + `"`
			var d trabbits.Duration
			if err := json.Unmarshal([]byte(jsonStr), &d); err != nil {
				t.Fatalf("Failed to unmarshal duration %s: %v", test.duration, err)
			}

			if d.ToDuration() != test.expected {
				t.Errorf("Expected duration %v, got %v", test.expected, d.ToDuration())
			}

			// Test marshaling back to JSON
			marshaled, err := json.Marshal(d)
			if err != nil {
				t.Fatalf("Failed to marshal duration: %v", err)
			}

			// Unmarshal again to verify round-trip
			var d2 trabbits.Duration
			if err := json.Unmarshal(marshaled, &d2); err != nil {
				t.Fatalf("Failed to unmarshal round-trip duration: %v", err)
			}

			if d2.ToDuration() != test.expected {
				t.Errorf("Round-trip duration mismatch: expected %v, got %v", test.expected, d2.ToDuration())
			}
		})
	}
}

func TestDurationInvalidFormats(t *testing.T) {
	invalidDurations := []string{
		"invalid",
		"30",  // missing unit
		"s30", // wrong format
		"",
	}

	for _, invalid := range invalidDurations {
		t.Run("invalid_"+invalid, func(t *testing.T) {
			jsonStr := `"` + invalid + `"`
			var d trabbits.Duration
			err := json.Unmarshal([]byte(jsonStr), &d)
			if err == nil {
				t.Errorf("Expected error for invalid duration %s, got nil", invalid)
			}
		})
	}
}

func TestConfigRoundTrip(t *testing.T) {
	// Create a config with health check settings
	original := &trabbits.Config{
		Upstreams: []trabbits.UpstreamConfig{
			{
				Name: "test-cluster",
				Cluster: &trabbits.ClusterConfig{
					Nodes: []string{
						"localhost:5672",
						"localhost:5673",
					},
				},
				Timeout: trabbits.Duration(10 * time.Second),
				HealthCheck: &trabbits.HealthCheckConfig{
					Interval:           trabbits.Duration(30 * time.Second),
					Timeout:            trabbits.Duration(5 * time.Second),
					UnhealthyThreshold: 3,
					RecoveryInterval:   trabbits.Duration(60 * time.Second),
					Username:           "admin",
					Password:           "admin",
				},
			},
		},
	}

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	// Write to temp file and load it back
	tmpfile, err := os.CreateTemp("", "test-config-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write(jsonData); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpfile.Close()

	// Load config from file
	loaded, err := trabbits.LoadConfig(t.Context(), tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify the loaded config matches the original
	if len(loaded.Upstreams) != len(original.Upstreams) {
		t.Errorf("Upstream count mismatch: expected %d, got %d", len(original.Upstreams), len(loaded.Upstreams))
	}

	loadedUpstream := loaded.Upstreams[0]
	originalUpstream := original.Upstreams[0]

	if loadedUpstream.Timeout.ToDuration() != originalUpstream.Timeout.ToDuration() {
		t.Errorf("Timeout mismatch: expected %v, got %v", originalUpstream.Timeout.ToDuration(), loadedUpstream.Timeout.ToDuration())
	}

	if loadedUpstream.HealthCheck.Interval.ToDuration() != originalUpstream.HealthCheck.Interval.ToDuration() {
		t.Errorf("Interval mismatch: expected %v, got %v", originalUpstream.HealthCheck.Interval.ToDuration(), loadedUpstream.HealthCheck.Interval.ToDuration())
	}
}
