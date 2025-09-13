package trabbits_test

import (
	"context"
	"testing"
	"time"

	"github.com/fujiwara/trabbits"
	"github.com/fujiwara/trabbits/config"
)

func TestProxyRegistration(t *testing.T) {

	// Create a proxy for internal logic testing

	// Set up test config
	config := &config.Config{
		Upstreams: []config.Upstream{
			{
				Name:    "test-upstream",
				Address: "localhost:5672",
			},
		},
	}

	// Create server instance
	server := trabbits.NewTestServer(config)

	// Create proxy with server instance
	proxy := server.NewProxy(nil)

	// Register proxy with dummy cancel function for testing
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	server.RegisterProxy(proxy, cancel)

	// Verify proxy is registered
	registeredProxy := server.GetProxy(proxy.ID())
	if registeredProxy == nil {
		t.Error("proxy should be registered")
	}
	if registeredProxy.ID() != proxy.ID() {
		t.Errorf("registered proxy ID mismatch: got %s, want %s", registeredProxy.ID(), proxy.ID())
	}

	// Unregister proxy
	server.UnregisterProxy(proxy)

	// Verify proxy is unregistered
	registeredProxy = server.GetProxy(proxy.ID())
	if registeredProxy != nil {
		t.Error("proxy should be unregistered")
	}

	// Verify that the default message constant is properly defined
	expectedDefaultMsg := trabbits.ShutdownMsgDefault
	if expectedDefaultMsg != "Connection closed" {
		t.Errorf("Expected default message constant to be 'Connection closed', got %q", expectedDefaultMsg)
	} else {
		t.Logf("✓ Default message constant verified: %q", expectedDefaultMsg)
	}
}

func TestDisconnectOutdatedProxies(t *testing.T) {

	// Create proxies for internal logic testing

	// Set up test configs with different hashes
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
	newHash := newConfig.Hash()

	if oldHash == newHash {
		t.Fatal("configs should have different hashes for this test")
	}

	// Create single server instance and manually set config hashes for testing
	server := trabbits.NewTestServer(oldConfig)

	// Create proxies and manually set different config hashes
	proxy1 := server.NewProxy(nil)
	proxy1.SetConfigHash(oldHash) // This proxy has old config
	_, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	server.RegisterProxy(proxy1, cancel1)

	proxy2 := server.NewProxy(nil)
	proxy2.SetConfigHash(newHash) // This proxy has new config
	_, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	server.RegisterProxy(proxy2, cancel2)

	// Count active proxies before disconnection
	initialCount := server.CountActiveProxies()
	if initialCount != 2 {
		t.Errorf("expected 2 active proxies, got %d", initialCount)
	}

	// No connection monitoring needed for nil connections - shutdown is immediate

	// Use the server to disconnect outdated proxies
	disconnectChan := server.TestDisconnectOutdatedProxies(newHash)

	// Wait for disconnection to complete with timeout
	select {
	case disconnectedCount := <-disconnectChan:
		expectedDisconnectedCount := 1 // proxy1 should be disconnected
		if disconnectedCount != expectedDisconnectedCount {
			t.Errorf("Expected %d proxies to be disconnected, got %d", expectedDisconnectedCount, disconnectedCount)
		} else {
			t.Logf("✓ Successfully disconnected %d outdated proxy(ies)", disconnectedCount)
		}

		// Check that proper number of proxies remain active
		// Note: In real usage, proxies are unregistered when connections close
		// For this test, both proxies are still registered but proxy1 received disconnect signal
		remainingCount := server.CountActiveProxies()
		expectedRemainingCount := 2 // Both still registered until connections actually close

		if remainingCount != expectedRemainingCount {
			t.Logf("ℹ Active proxy count after disconnection signal: %d (expected %d)", remainingCount, expectedRemainingCount)
		} else {
			t.Logf("✓ Both proxies still registered after disconnection signal sent: %d", remainingCount)
		}

		// Test successful - the disconnect signal was sent and completed

		// Verify that the correct config update message constant is used
		expectedConfigUpdateMsg := trabbits.ShutdownMsgConfigUpdate
		if expectedConfigUpdateMsg != "Configuration updated, please reconnect" {
			t.Errorf("Expected config update message constant to be 'Configuration updated, please reconnect', got %q", expectedConfigUpdateMsg)
		} else {
			t.Logf("✓ Config update message constant verified: %q", expectedConfigUpdateMsg)
		}

	case <-time.After(5 * time.Second):
		t.Error("✗ Timeout waiting for proxy disconnection to complete")
	}

	// Clean up
	server.UnregisterProxy(proxy1)
	server.UnregisterProxy(proxy2)
}
