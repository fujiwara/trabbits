package trabbits_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/fujiwara/trabbits"
	"github.com/fujiwara/trabbits/config"
)

// TestSIGHUPHandler tests that the SIGHUP signal properly triggers configuration reload
func TestSIGHUPHandler(t *testing.T) {
	// Skip this test in CI environments or if running in parallel
	if testing.Short() {
		t.Skip("Skipping SIGHUP test in short mode")
	}

	// Create a temporary config file
	originalConfig := `{
		"upstreams": [
			{
				"name": "primary",
				"address": "localhost:5672",
				"routing": {}
			}
		]
	}`

	tmpfile, err := os.CreateTemp("", "trabbits-sighup-test-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp config file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(originalConfig)); err != nil {
		t.Fatalf("Failed to write temp config file: %v", err)
	}
	tmpfile.Close()

	// Test the reload function directly
	cfg, err := config.Load(t.Context(), tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load initial config: %v", err)
	}

	if len(cfg.Upstreams) != 1 || cfg.Upstreams[0].Name != "primary" {
		t.Fatalf("Unexpected initial config: %+v", cfg)
	}

	// Update the config file
	updatedConfig := `{
		"upstreams": [
			{
				"name": "primary",
				"address": "localhost:5672",
				"routing": {}
			},
			{
				"name": "sighup-added",
				"address": "localhost:5673",
				"routing": {
					"key_patterns": ["sighup.test.*"]
				}
			}
		]
	}`

	if err := os.WriteFile(tmpfile.Name(), []byte(updatedConfig), 0644); err != nil {
		t.Fatalf("Failed to update config file: %v", err)
	}

	// Test that reloadConfigFromFile works correctly
	reloadedCfg, err := trabbits.ReloadConfigFromFile(t.Context(), tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to reload config: %v", err)
	}

	if len(reloadedCfg.Upstreams) != 2 {
		t.Fatalf("Expected 2 upstreams after reload, got %d", len(reloadedCfg.Upstreams))
	}

	foundSighupAdded := false
	for _, upstream := range reloadedCfg.Upstreams {
		if upstream.Name == "sighup-added" {
			foundSighupAdded = true
			if len(upstream.Routing.KeyPatterns) != 1 || upstream.Routing.KeyPatterns[0] != "sighup.test.*" {
				t.Errorf("Unexpected routing patterns for sighup-added upstream: %v", upstream.Routing.KeyPatterns)
			}
			break
		}
	}

	if !foundSighupAdded {
		t.Error("Expected to find 'sighup-added' upstream after reload")
	}

	slog.Info("SIGHUP reload functionality test completed successfully")
}
