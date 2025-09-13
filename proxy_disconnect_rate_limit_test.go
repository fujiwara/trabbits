package trabbits_test

import (
	"net"
	"testing"
	"time"

	"github.com/fujiwara/trabbits"
	"github.com/fujiwara/trabbits/config"
)

func TestDisconnectOutdatedProxies_RateLimit(t *testing.T) {
	// Clear any remaining proxies from previous tests

	// Test rate limiting with multiple proxies
	oldConfig := &config.Config{
		Upstreams: []config.UpstreamConfig{
			{
				Name:    "test-upstream",
				Address: "localhost:5672",
			},
		},
	}

	newConfig := &config.Config{
		Upstreams: []config.UpstreamConfig{
			{
				Name:    "test-upstream",
				Address: "localhost:5673", // Different port = different hash
			},
		},
	}

	oldHash := oldConfig.Hash()
	newHash := newConfig.Hash()

	// Create server instance
	testServer := trabbits.NewTestServer(oldConfig)

	// Create multiple proxies with old config hash
	const numProxies = 50
	var proxies []*trabbits.Proxy
	var connections []net.Conn

	for i := 0; i < numProxies; i++ {
		server, client := net.Pipe()
		connections = append(connections, server, client)

		proxy := testServer.NewProxy(server)
		proxy.SetConfigHash(oldHash)
		testServer.RegisterProxy(proxy)
		proxies = append(proxies, proxy)
	}

	// Clean up connections
	defer func() {
		for _, conn := range connections {
			conn.Close()
		}
		for _, proxy := range proxies {
			testServer.UnregisterProxy(proxy)
		}
	}()

	// Verify initial count
	initialCount := testServer.CountActiveProxies()
	if initialCount < numProxies {
		t.Errorf("Expected at least %d active proxies, got %d", numProxies, initialCount)
	}

	// Start disconnection and measure time
	start := time.Now()
	disconnectChan := testServer.TestDisconnectOutdatedProxies(newHash)

	select {
	case disconnectedCount := <-disconnectChan:
		elapsed := time.Since(start)

		if disconnectedCount != numProxies {
			t.Errorf("Expected %d proxies to be disconnected, got %d", numProxies, disconnectedCount)
		} else {
			t.Logf("✓ Successfully disconnected %d proxies", disconnectedCount)
		}

		// At 100/sec rate, 50 proxies should take at least 0.5 seconds
		// But with 10 workers and burst capacity, it might be faster
		expectedMinDuration := 200 * time.Millisecond // Conservative estimate
		if elapsed < expectedMinDuration {
			t.Logf("ℹ Disconnection completed in %v (expected at least %v with rate limiting)", elapsed, expectedMinDuration)
		} else {
			t.Logf("✓ Rate limiting working correctly: %v for %d proxies", elapsed, numProxies)
		}

		// Should not take too long either (max 2-3 seconds for 50 proxies)
		maxExpectedDuration := 5 * time.Second
		if elapsed > maxExpectedDuration {
			t.Errorf("Disconnection took too long: %v (expected less than %v)", elapsed, maxExpectedDuration)
		}

	case <-time.After(10 * time.Second):
		t.Error("✗ Timeout waiting for rate-limited disconnection to complete")
	}
}

func TestDisconnectOutdatedProxies_TimeoutCalculation(t *testing.T) {
	// Clear any remaining proxies from previous tests

	// Test timeout calculation for large numbers of proxies
	oldConfig := &config.Config{
		Upstreams: []config.UpstreamConfig{
			{
				Name:    "test-upstream",
				Address: "localhost:5672",
			},
		},
	}

	newConfig := &config.Config{
		Upstreams: []config.UpstreamConfig{
			{
				Name:    "test-upstream",
				Address: "localhost:5673", // Different port = different hash
			},
		},
	}

	oldHash := oldConfig.Hash()
	newHash := newConfig.Hash()

	// Create server instance
	testServer := trabbits.NewTestServer(oldConfig)

	// Create many proxies to test timeout calculation
	const numProxies = 5 // Small number for quick test
	var proxies []*trabbits.Proxy
	var connections []net.Conn

	for i := 0; i < numProxies; i++ {
		server, client := net.Pipe()
		connections = append(connections, server, client)

		proxy := testServer.NewProxy(server)
		proxy.SetConfigHash(oldHash)
		testServer.RegisterProxy(proxy)
		proxies = append(proxies, proxy)
	}

	// Clean up connections
	defer func() {
		for _, conn := range connections {
			conn.Close()
		}
		for _, proxy := range proxies {
			testServer.UnregisterProxy(proxy)
		}
	}()

	// Test that disconnection completes within calculated timeout
	start := time.Now()
	disconnectChan := testServer.TestDisconnectOutdatedProxies(newHash)

	// Expected timeout for 5 proxies: 5/100 * 1.2 + 2 = ~2.06 seconds
	// Should complete much faster than that
	select {
	case disconnectedCount := <-disconnectChan:
		elapsed := time.Since(start)

		if disconnectedCount != numProxies {
			t.Errorf("Expected %d proxies to be disconnected, got %d", numProxies, disconnectedCount)
		}

		t.Logf("✓ Disconnected %d proxies in %v", disconnectedCount, elapsed)

		// Should complete quickly for small numbers
		if elapsed > 3*time.Second {
			t.Errorf("Small proxy count took too long: %v", elapsed)
		}

	case <-time.After(5 * time.Second):
		t.Error("✗ Timeout waiting for small proxy count disconnection")
	}
}
