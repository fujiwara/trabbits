package trabbits_test

import (
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/fujiwara/trabbits"
	"github.com/fujiwara/trabbits/config"
)

func TestConfigUpdateDisconnectsOutdatedProxies(t *testing.T) {

	// Capture log output to verify disconnection messages
	var logOutput strings.Builder

	// Create a test logger that writes to our buffer
	logger := slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	// Create test config
	oldConfig := &config.Config{
		Upstreams: []config.Upstream{
			{
				Name:    "test-upstream",
				Address: "localhost:5672",
			},
		},
	}

	newConfig := &config.Config{
		Upstreams: []config.Upstream{
			{
				Name:    "test-upstream",
				Address: "localhost:5673", // Different port = different hash
			},
		},
	}

	oldHash := oldConfig.Hash()

	// Create server instance
	testServer := trabbits.NewTestServer(oldConfig)

	// Create a mock proxy with old config hash
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	proxy := testServer.NewProxy(server)
	proxy.SetConfigHash(oldHash)
	testServer.RegisterProxy(proxy)

	// Update to new config
	newHash := newConfig.Hash()

	// Clear log buffer
	logOutput.Reset()

	// Trigger disconnection of outdated proxies and wait for completion
	disconnectChan := testServer.TestDisconnectOutdatedProxies(newHash)

	// Wait for disconnection to complete
	select {
	case disconnectedCount := <-disconnectChan:
		expectedCount := 1 // One proxy should be disconnected
		if disconnectedCount != expectedCount {
			t.Errorf("Expected %d proxies to be disconnected, got %d", expectedCount, disconnectedCount)
		} else {
			t.Logf("✓ Successfully disconnected %d proxy(ies) with outdated config", disconnectedCount)
		}
	case <-time.After(3 * time.Second):
		t.Error("✗ Timeout waiting for proxy disconnection to complete")
		return
	}

	// Check log output for disconnection messages
	logStr := logOutput.String()

	if !strings.Contains(logStr, "Disconnecting outdated proxies") {
		t.Error("Expected 'Disconnecting outdated proxies' log message not found")
	}

	if !strings.Contains(logStr, "Disconnecting proxy due to config update") {
		t.Error("Expected 'Disconnecting proxy due to config update' log message not found")
	}

	if !strings.Contains(logStr, oldHash[:8]) {
		t.Errorf("Expected old hash %s in log output", oldHash[:8])
	}

	if !strings.Contains(logStr, newHash[:8]) {
		t.Errorf("Expected new hash %s in log output", newHash[:8])
	}

	t.Logf("✓ Disconnection process logged correctly:\n%s", logStr)

	// Clean up
	testServer.UnregisterProxy(proxy)
}
