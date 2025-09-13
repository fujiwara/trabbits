package trabbits_test

import (
	"net"
	"testing"
	"time"

	"github.com/fujiwara/trabbits"
	"github.com/fujiwara/trabbits/config"
)

func TestDisconnectOutdatedProxies_ZeroProxies(t *testing.T) {
	t.Parallel()

	// Create test config
	config := &config.Config{
		Upstreams: []config.Upstream{
			{
				Name:    "test-upstream",
				Address: "localhost:5672",
			},
		},
	}
	configHash := config.Hash()

	// Create server instance
	testServer := trabbits.NewTestServer(config)

	// Create proxy with CURRENT config hash (not outdated)
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	proxy := testServer.NewProxy(server)
	proxy.SetConfigHash(configHash) // Same hash as current config
	testServer.RegisterProxy(proxy)

	// Disconnect outdated proxies - should find 0 because proxy has current hash
	start := time.Now()
	disconnectChan := testServer.TestDisconnectOutdatedProxies(configHash)

	select {
	case disconnectedCount := <-disconnectChan:
		elapsed := time.Since(start)

		// Should return 0 immediately
		expectedCount := 0
		if disconnectedCount != expectedCount {
			t.Errorf("Expected %d proxies to be disconnected, got %d", expectedCount, disconnectedCount)
		} else {
			t.Logf("✓ Correctly returned %d disconnected proxies", disconnectedCount)
		}

		// Should complete very quickly (much less than timeout)
		if elapsed > 100*time.Millisecond {
			t.Errorf("Took too long for zero proxy case: %v", elapsed)
		} else {
			t.Logf("✓ Completed quickly in %v (expected fast return)", elapsed)
		}

	case <-time.After(1 * time.Second):
		t.Error("✗ Timeout waiting for zero proxy disconnection (should be immediate)")
	}

	// Verify proxy is still active
	finalCount := testServer.CountActiveProxies()
	if finalCount != 1 {
		t.Errorf("Expected 1 active proxy remaining, got %d", finalCount)
	}

	// Clean up
	testServer.UnregisterProxy(proxy)
}

func TestDisconnectOutdatedProxies_NoProxies(t *testing.T) {
	t.Parallel()

	// Test with no proxies registered at all
	config := &config.Config{
		Upstreams: []config.Upstream{
			{
				Name:    "test-upstream",
				Address: "localhost:5672",
			},
		},
	}

	// Create server instance
	testServer := trabbits.NewTestServer(config)

	start := time.Now()
	disconnectChan := testServer.TestDisconnectOutdatedProxies(config.Hash())

	select {
	case disconnectedCount := <-disconnectChan:
		elapsed := time.Since(start)

		// Should return 0 immediately
		if disconnectedCount != 0 {
			t.Errorf("Expected 0 proxies to be disconnected, got %d", disconnectedCount)
		} else {
			t.Logf("✓ Correctly returned %d disconnected proxies for empty proxy list", disconnectedCount)
		}

		// Should complete very quickly
		if elapsed > 50*time.Millisecond {
			t.Errorf("Took too long for no proxy case: %v", elapsed)
		} else {
			t.Logf("✓ Completed quickly in %v (expected immediate return)", elapsed)
		}

	case <-time.After(1 * time.Second):
		t.Error("✗ Timeout waiting for no proxy disconnection (should be immediate)")
	}
}
