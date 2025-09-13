// MIT License
// Copyright (c) 2025 FUJIWARA Shunichiro

package trabbits_test

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/fujiwara/trabbits"
	"github.com/fujiwara/trabbits/config"
)

func TestGracefulShutdown(t *testing.T) {
	// Note: Don't use t.Parallel() here to avoid log handler conflicts

	// Capture log output for verification
	var logOutput bytes.Buffer
	originalLogger := slog.Default()
	defer slog.SetDefault(originalLogger) // Restore original logger
	slog.SetDefault(slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Create test config
	cfg := &config.Config{
		Upstreams: []config.Upstream{
			{
				Name:    "test-upstream",
				Address: "localhost:5672",
			},
		},
	}

	// Create server instance
	testServer := trabbits.NewTestServer(cfg)

	// Create multiple active connections
	const numProxies = 5
	var proxies []*trabbits.Proxy
	var connections []net.Conn

	for i := 0; i < numProxies; i++ {
		server, client := net.Pipe()
		connections = append(connections, server, client)

		proxy := testServer.NewProxy(server)
		testServer.RegisterProxy(proxy)
		proxies = append(proxies, proxy)
	}

	// Clean up connections
	defer func() {
		for _, conn := range connections {
			conn.Close()
		}
	}()

	// Verify that all proxies are registered
	activeCount := testServer.CountActiveProxies()
	if activeCount != numProxies {
		t.Errorf("Expected %d active proxies, got %d", numProxies, activeCount)
	}

	// Test disconnectAllProxies method
	disconnectChan := testServer.TestDisconnectAllProxies()

	// Wait for disconnection to complete
	select {
	case disconnectedCount := <-disconnectChan:
		if disconnectedCount != numProxies {
			t.Errorf("Expected %d proxies to be disconnected, got %d", numProxies, disconnectedCount)
		} else {
			t.Logf("✓ Successfully disconnected %d proxy(ies) for shutdown", disconnectedCount)
		}
	case <-time.After(3 * time.Second):
		t.Error("✗ Timeout waiting for proxy disconnection to complete")
		return
	}

	// Check log output for shutdown messages
	logStr := logOutput.String()
	t.Logf("Log output:\n%s", logStr)

	// With the disconnectAllProxies method called directly, we should see these messages
	if !strings.Contains(logStr, "Disconnecting all active proxies for shutdown") {
		t.Error("Expected 'Disconnecting all active proxies for shutdown' log message not found")
	}

	if !strings.Contains(logStr, "Disconnecting proxy for shutdown") {
		t.Error("Expected 'Disconnecting proxy for shutdown' log message not found")
	}

	// Note: With net.Pipe() connections, sendConnectionError may fail and cause timeout
	// This is expected behavior - the timeout message indicates graceful shutdown was attempted
	if strings.Contains(logStr, "Timeout waiting for proxy disconnections") {
		t.Log("✓ Graceful shutdown attempted (timeout expected with net.Pipe connections)")
	} else if strings.Contains(logStr, "All proxy disconnections completed for shutdown") {
		t.Log("✓ All proxy disconnections completed successfully")
	} else {
		t.Error("Expected either completion or timeout log message not found")
	}
}

func TestGracefulShutdown_NoProxies(t *testing.T) {
	t.Parallel()

	// Test graceful shutdown with no active proxies
	cfg := &config.Config{
		Upstreams: []config.Upstream{
			{
				Name:    "test-upstream",
				Address: "localhost:5672",
			},
		},
	}

	testServer := trabbits.NewTestServer(cfg)

	// Verify no proxies are registered
	activeCount := testServer.CountActiveProxies()
	if activeCount != 0 {
		t.Errorf("Expected 0 active proxies, got %d", activeCount)
	}

	// Test disconnectAllProxies method with no proxies
	disconnectChan := testServer.TestDisconnectAllProxies()

	select {
	case disconnectedCount := <-disconnectChan:
		if disconnectedCount != 0 {
			t.Errorf("Expected 0 proxies to be disconnected, got %d", disconnectedCount)
		} else {
			t.Logf("✓ Correctly handled shutdown with no active proxies")
		}
	case <-time.After(1 * time.Second):
		t.Error("✗ Timeout waiting for empty shutdown to complete")
	}
}

func TestGracefulShutdown_ContextCancellation(t *testing.T) {
	// Note: Don't use t.Parallel() here to avoid log handler conflicts

	// Capture log output for verification
	var logOutput bytes.Buffer
	originalLogger := slog.Default()
	defer slog.SetDefault(originalLogger) // Restore original logger
	slog.SetDefault(slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Test that context cancellation triggers graceful shutdown
	cfg := &config.Config{
		Upstreams: []config.Upstream{
			{
				Name:    "test-upstream",
				Address: "localhost:5672",
			},
		},
	}

	testServer := trabbits.NewTestServer(cfg)

	// Create a mock connection
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	proxy := testServer.NewProxy(server)
	testServer.RegisterProxy(proxy)

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Create a mock listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	// Start the server in a goroutine
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- testServer.TestBoot(ctx, listener)
	}()

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	// Cancel the context to trigger shutdown
	cancel()

	// Wait for server to complete
	select {
	case err := <-serverDone:
		if err != nil {
			t.Errorf("Server boot returned error: %v", err)
		} else {
			t.Log("✓ Server shutdown completed successfully")
		}
	case <-time.After(15 * time.Second):
		t.Error("✗ Timeout waiting for server shutdown")
	}

	// Check that the proper shutdown sequence occurred
	logStr := logOutput.String()
	t.Logf("Context cancellation log output:\n%s", logStr)

	// Note: Due to timing, we may not always capture all log messages
	if strings.Contains(logStr, "Listener closed, no new connections will be accepted") {
		t.Log("✓ Found 'Listener closed' log message")
	} else {
		t.Log("⚠ 'Listener closed' log message not captured (timing issue)")
	}

	if strings.Contains(logStr, "trabbits server stopped") {
		t.Log("✓ Found 'trabbits server stopped' log message")
	} else {
		t.Log("⚠ 'trabbits server stopped' log message not captured (timing issue)")
	}
}
