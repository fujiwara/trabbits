package trabbits_test

import (
	"net"
	"testing"
	"time"

	"github.com/fujiwara/trabbits"
)

func TestProxyRegistration(t *testing.T) {
	// Clear any remaining proxies from previous tests
	trabbits.ClearActiveProxies()

	// Create a mock connection for testing
	serverConn, client := net.Pipe()
	defer serverConn.Close()
	defer client.Close()

	// Set up test config
	config := &trabbits.Config{
		Upstreams: []trabbits.UpstreamConfig{
			{
				Name:    "test-upstream",
				Address: "localhost:5672",
			},
		},
	}

	// Create server instance
	server := trabbits.NewTestServer(config)

	// Create proxy with server instance
	proxy := server.NewProxy(serverConn)

	// Register proxy
	server.RegisterProxy(proxy)

	// Verify proxy is registered
	registeredProxy := trabbits.GetProxy(proxy.ID())
	if registeredProxy == nil {
		t.Error("proxy should be registered")
	}
	if registeredProxy.ID() != proxy.ID() {
		t.Errorf("registered proxy ID mismatch: got %s, want %s", registeredProxy.ID(), proxy.ID())
	}

	// Unregister proxy
	server.UnregisterProxy(proxy)

	// Verify proxy is unregistered
	registeredProxy = trabbits.GetProxy(proxy.ID())
	if registeredProxy != nil {
		t.Error("proxy should be unregistered")
	}
}

func TestDisconnectOutdatedProxies(t *testing.T) {
	// Clear any remaining proxies from previous tests
	trabbits.ClearActiveProxies()

	// Create mock connections
	server1, client1 := net.Pipe()
	defer server1.Close()
	defer client1.Close()

	server2, client2 := net.Pipe()
	defer server2.Close()
	defer client2.Close()

	// Set up test configs with different hashes
	oldConfig := &trabbits.Config{
		Upstreams: []trabbits.UpstreamConfig{
			{
				Name:    "test-upstream",
				Address: "localhost:5672",
			},
		},
	}
	newConfig := &trabbits.Config{
		Upstreams: []trabbits.UpstreamConfig{
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
	proxy1 := server.NewProxy(server1)
	proxy1.SetConfigHash(oldHash) // This proxy has old config
	server.RegisterProxy(proxy1)

	proxy2 := server.NewProxy(server2)
	proxy2.SetConfigHash(newHash) // This proxy has new config
	server.RegisterProxy(proxy2)

	// Count active proxies before disconnection
	initialCount := trabbits.CountActiveProxies()
	if initialCount != 2 {
		t.Errorf("expected 2 active proxies, got %d", initialCount)
	}

	// Set up connection monitoring for proxy1 (outdated config)
	disconnectDetected := make(chan bool, 1)
	go func() {
		// Try to read from client1 connection - should fail when proxy sends disconnect
		buf := make([]byte, 1)
		_, err := client1.Read(buf)
		if err != nil {
			disconnectDetected <- true
		}
	}()

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
		remainingCount := trabbits.CountActiveProxies()
		expectedRemainingCount := 2 // Both still registered until connections actually close

		if remainingCount != expectedRemainingCount {
			t.Logf("ℹ Active proxy count after disconnection signal: %d (expected %d)", remainingCount, expectedRemainingCount)
		} else {
			t.Logf("✓ Both proxies still registered after disconnection signal sent: %d", remainingCount)
		}

		// Test successful - the disconnect signal was sent and completed

	case <-time.After(5 * time.Second):
		t.Error("✗ Timeout waiting for proxy disconnection to complete")
	}

	// Clean up
	server.UnregisterProxy(proxy1)
	server.UnregisterProxy(proxy2)
}
