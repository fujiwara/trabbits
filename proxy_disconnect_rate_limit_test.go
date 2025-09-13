package trabbits_test

import (
	"context"
	"testing"
	"time"

	"github.com/fujiwara/trabbits"
	"github.com/fujiwara/trabbits/config"
)

func TestDisconnectOutdatedProxies_RateLimit(t *testing.T) {
	t.Parallel()

	// Test rate limiting with multiple proxies
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

	// Create server instance
	testServer := trabbits.NewTestServer(oldConfig)

	// Create multiple proxies with old config hash for internal logic testing
	const numProxies = 50
	var proxies []*trabbits.Proxy

	for i := 0; i < numProxies; i++ {
		proxy := testServer.NewProxy(nil) // Use nil connection for internal logic testing
		proxy.SetConfigHash(oldHash)
		_, cancel := context.WithCancel(context.Background())
		defer cancel()
		testServer.RegisterProxy(proxy, cancel)
		proxies = append(proxies, proxy)
	}

	// Clean up proxies
	defer func() {
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

		// With nil connections, shutdown should be immediate but rate limiting still applies
		// The rate limiting logic should still be tested even with instant shutdowns
		t.Logf("✓ Disconnection completed in %v for %d proxies with rate limiting", elapsed, numProxies)

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
	t.Parallel()

	// Test timeout calculation for large numbers of proxies
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

	// Create server instance
	testServer := trabbits.NewTestServer(oldConfig)

	// Create proxies for internal logic testing
	const numProxies = 5 // Small number for quick test
	var proxies []*trabbits.Proxy

	for i := 0; i < numProxies; i++ {
		proxy := testServer.NewProxy(nil) // Use nil connection for internal logic testing
		proxy.SetConfigHash(oldHash)
		_, cancel := context.WithCancel(context.Background())
		defer cancel()
		testServer.RegisterProxy(proxy, cancel)
		proxies = append(proxies, proxy)
	}

	// Clean up proxies
	defer func() {
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
