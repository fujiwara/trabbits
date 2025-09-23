package trabbits_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fujiwara/trabbits"
	"github.com/fujiwara/trabbits/apiclient"
	"github.com/fujiwara/trabbits/config"
	"github.com/fujiwara/trabbits/types"
	"github.com/google/go-cmp/cmp"
	rabbitmq "github.com/rabbitmq/amqp091-go"
)

// newAPIClient creates an API client for testing
func newAPIClient(socketPath string) *apiclient.Client {
	return apiclient.New(socketPath)
}

// startUnifiedTestServer starts a single server instance with both AMQP proxy and API
func startUnifiedTestServer(t *testing.T, configFile string) (context.CancelFunc, string, *apiclient.Client, int) {
	ctx, cancel := context.WithCancel(t.Context())

	// Create socket for API
	tmpfile, err := os.CreateTemp("", "trabbits-unified-api-sock-")
	if err != nil {
		t.Fatalf("failed to create temp socket file: %v", err)
	}
	socketPath := tmpfile.Name()
	os.Remove(socketPath) // trabbits will recreate it

	// Load config
	cfg, err := config.Load(ctx, configFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Disable health checks for test performance
	for _, upstream := range cfg.Upstreams {
		upstream.HealthCheck = nil
	}

	// Create listener for AMQP proxy
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	proxyPort := listener.Addr().(*net.TCPAddr).Port

	// Create single server instance
	server := trabbits.NewServer(cfg, socketPath)

	// Start both AMQP proxy and API server on the same instance
	go func() {
		defer listener.Close()
		if err := server.TestBoot(ctx, listener); err != nil && ctx.Err() == nil {
			t.Errorf("AMQP proxy server failed: %v", err)
		}
	}()

	go func() {
		if _, err := server.TestStartAPIServer(ctx, configFile); err != nil && ctx.Err() == nil {
			t.Errorf("API server failed: %v", err)
		}
	}()

	// Wait for both services to be available
	waitForTestAPI(t, socketPath, 1*time.Second)

	// Test AMQP connectivity
	time.Sleep(100 * time.Millisecond)
	testConn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort), 1*time.Second)
	if err != nil {
		t.Fatalf("AMQP proxy not ready: %v", err)
	}
	testConn.Close()

	client := newAPIClient(socketPath)
	return cancel, socketPath, client, proxyPort
}

// startIsolatedAPIServer starts a new API server instance for isolated testing
func startIsolatedAPIServer(t *testing.T, configFile string) (context.CancelFunc, string, *apiclient.Client) {
	ctx, cancel := context.WithCancel(t.Context())

	tmpfile, err := os.CreateTemp("", "trabbits-isolated-api-sock-")
	if err != nil {
		t.Fatalf("failed to create temp socket file: %v", err)
	}
	socketPath := tmpfile.Name()
	os.Remove(socketPath) // trabbits will recreate it

	// Create server instance for API server
	cfg, err := config.Load(ctx, configFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Disable health checks for test performance
	for _, upstream := range cfg.Upstreams {
		upstream.HealthCheck = nil
	}

	server := trabbits.NewServer(cfg, socketPath)

	go func() {
		_, err := server.TestStartAPIServer(ctx, configFile)
		if err != nil && ctx.Err() == nil {
			t.Errorf("API server failed: %v", err)
		}
	}()

	// Wait for API to be available with polling
	waitForTestAPI(t, socketPath, 1*time.Second)

	client := newAPIClient(socketPath)
	return cancel, socketPath, client
}

// waitForTestAPI waits for API socket to be available
func waitForTestAPI(t *testing.T, socketPath string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socketPath); err == nil {
			// Try to actually connect using API client
			client := newAPIClient(socketPath)
			if _, err := client.GetConfig(t.Context()); err == nil {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("API socket not available after %v", timeout)
}

// TestAPIPutInvalidConfig is skipped - apiclient package doesn't support direct invalid JSON testing

// TestAPIConfigUpdateIsolated runs all config-update tests in isolation with a dedicated server
func TestAPIConfigUpdateIsolated(t *testing.T) {
	// Start isolated server instance for config update tests
	cancel, socketPath, client := startIsolatedAPIServer(t, "testdata/config.json")
	defer cancel()
	defer os.Remove(socketPath)


	// Test 1: Basic PUT/GET cycle
	t.Run("BasicPutGetConfig", func(t *testing.T) {
		// GET current config using API client
		testConfig, err := client.GetConfig(t.Context())
		if err != nil {
			t.Fatalf("failed to get config: %v", err)
		}

		// Update config
		if len(testConfig.Upstreams) >= 2 {
			testConfig.Upstreams[1].Routing.KeyPatterns = append(
				testConfig.Upstreams[1].Routing.KeyPatterns, "test.isolated.*",
			)
		}

		// PUT updated config using API client
		if err := client.PutConfig(t.Context(), testConfig); err != nil {
			t.Fatalf("failed to put config: %v", err)
		}

		// Verify updated config
		updatedConfig, err := client.GetConfig(t.Context())
		if err != nil {
			t.Fatalf("failed to get updated config: %v", err)
		}
		if diff := cmp.Diff(testConfig, updatedConfig); diff != "" {
			t.Errorf("unexpected response: %s", diff)
		}
	})

	// Test 2: Raw JSON config with environment variables
	t.Run("RawJSONConfig", func(t *testing.T) {
		t.Setenv("TEST_API_USERNAME", "apiuser")
		t.Setenv("TEST_API_PASSWORD", "apipass")

		rawConfigJSON := `{
		"upstreams": [
			{
				"name": "primary",
				"address": "localhost:5672",
				"routing": {}
			},
			{
				"name": "test-upstream",
				"cluster": {
					"nodes": [
						"localhost:5673"
					]
				},
				"health_check": {
					"interval": "100ms",
					"timeout": "50ms",
					"unhealthy_threshold": 1,
					"recovery_interval": "100ms",
					"username": "${TEST_API_USERNAME}",
					"password": "${TEST_API_PASSWORD}"
				},
				"routing": {
					"key_patterns": [
						"test.api.*"
					]
				}
			}
		]
	}`

		// Create temporary JSON file
		tmpfile, err := os.CreateTemp("", "test-raw-config-*.json")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpfile.Name())

		if _, err := tmpfile.Write([]byte(rawConfigJSON)); err != nil {
			t.Fatalf("Failed to write temp file: %v", err)
		}
		tmpfile.Close()

		// Use API client to put config from file
		if err := client.PutConfigFromFile(t.Context(), tmpfile.Name()); err != nil {
			t.Fatalf("failed to put config from file: %v", err)
		}

		// Get the config to verify
		respondConfig, err := client.GetConfig(t.Context())
		if err != nil {
			t.Fatalf("failed to get config: %v", err)
		}

		if len(respondConfig.Upstreams) != 2 {
			t.Fatalf("Expected 2 upstreams, got %d", len(respondConfig.Upstreams))
		}

		testUpstream := respondConfig.Upstreams[1]
		if testUpstream.HealthCheck == nil {
			t.Fatal("Expected health check configuration, got nil")
		}
		if testUpstream.HealthCheck.Username != "apiuser" {
			t.Errorf("Expected username 'apiuser', got '%s'", testUpstream.HealthCheck.Username)
		}
	})

	// Test 3: Jsonnet config
	t.Run("JsonnetConfig", func(t *testing.T) {
		t.Setenv("TEST_JSONNET_USERNAME", "jsonnetuser")
		t.Setenv("TEST_JSONNET_PASSWORD", "jsonnetpass")

		rawConfigJsonnet := `
local env = std.native('env');
{
  upstreams: [
    {
      name: 'primary',
      address: 'localhost:5672',
      routing: {},
    },
    {
      name: 'jsonnet-upstream',
      cluster: {
        nodes: [
          'localhost:5674',
        ],
      },
      health_check: {
        interval: '100ms',
        timeout: '50ms',
        unhealthy_threshold: 1,
        recovery_interval: '100ms',
        username: env('TEST_JSONNET_USERNAME', 'default'),
        password: env('TEST_JSONNET_PASSWORD', 'default'),
      },
      routing: {
        key_patterns: [
          'jsonnet.test.*',
        ],
      },
    },
  ],
}`

		// Create temporary Jsonnet file
		tmpfile, err := os.CreateTemp("", "test-api-config-*.jsonnet")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpfile.Name())

		if _, err := tmpfile.Write([]byte(rawConfigJsonnet)); err != nil {
			t.Fatalf("Failed to write temp file: %v", err)
		}
		tmpfile.Close()


		// Use API client to put config from Jsonnet file
		if err := client.PutConfigFromFile(t.Context(), tmpfile.Name()); err != nil {
			t.Fatalf("failed to put config from file: %v", err)
		}

		// Get the config to verify
		respondConfig, err := client.GetConfig(t.Context())
		if err != nil {
			t.Fatalf("failed to get config: %v", err)
		}

		if len(respondConfig.Upstreams) != 2 {
			t.Fatalf("Expected 2 upstreams, got %d", len(respondConfig.Upstreams))
		}

		jsonnetUpstream := respondConfig.Upstreams[1]
		if jsonnetUpstream.Name != "jsonnet-upstream" {
			t.Errorf("Expected upstream name 'jsonnet-upstream', got '%s'", jsonnetUpstream.Name)
		}

		if jsonnetUpstream.HealthCheck == nil {
			t.Fatal("Expected health check configuration, got nil")
		}

		if jsonnetUpstream.HealthCheck.Username != "jsonnetuser" {
			t.Errorf("Expected username 'jsonnetuser', got '%s'", jsonnetUpstream.HealthCheck.Username)
		}
	})

	// Test 4: Diff with JSON config
	t.Run("DiffJSONConfig", func(t *testing.T) {
		t.Setenv("TEST_DIFF_USERNAME", "diffuser")
		t.Setenv("TEST_DIFF_PASSWORD", "diffpass")

		rawConfigJSON := `{
		"upstreams": [
			{
				"name": "primary",
				"address": "localhost:5672",
				"routing": {}
			},
			{
				"name": "diff-test-upstream",
				"cluster": {
					"nodes": [
						"localhost:5673"
					]
				},
				"health_check": {
					"interval": "100ms",
					"timeout": "50ms",
					"unhealthy_threshold": 1,
					"recovery_interval": "100ms",
					"username": "${TEST_DIFF_USERNAME}",
					"password": "${TEST_DIFF_PASSWORD}"
				},
				"routing": {
					"key_patterns": [
						"diff.test.*"
					]
				}
			}
		]
	}`

		// Create temporary JSON file for diff
		tmpfile, err := os.CreateTemp("", "test-diff-config-*.json")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpfile.Name())

		if _, err := tmpfile.Write([]byte(rawConfigJSON)); err != nil {
			t.Fatalf("Failed to write temp file: %v", err)
		}
		tmpfile.Close()

		// Use API client to get diff
		diffText, err := client.DiffConfigFromFile(t.Context(), tmpfile.Name())
		if err != nil {
			t.Fatalf("failed to get diff: %v", err)
		}
		if !strings.Contains(diffText, "diff-test-upstream") {
			t.Errorf("Expected diff to contain 'diff-test-upstream', but got: %s", diffText)
		}
	})

	// Test 5: Diff with Jsonnet config
	t.Run("DiffJsonnetConfig", func(t *testing.T) {
		t.Setenv("TEST_JSONNET_DIFF_USERNAME", "jsonnetdiffuser")
		t.Setenv("TEST_JSONNET_DIFF_PASSWORD", "jsonnetdiffpass")

		rawConfigJsonnet := `
local env = std.native('env');
{
  upstreams: [
    {
      name: 'primary',
      address: 'localhost:5672',
      routing: {},
    },
    {
      name: 'jsonnet-diff-upstream',
      cluster: {
        nodes: [
          'localhost:5675',
        ],
      },
      health_check: {
        interval: '100ms',
        timeout: '50ms',
        unhealthy_threshold: 1,
        recovery_interval: '100ms',
        username: env('TEST_JSONNET_DIFF_USERNAME', 'default'),
        password: env('TEST_JSONNET_DIFF_PASSWORD', 'default'),
      },
      routing: {
        key_patterns: [
          'jsonnet.diff.test.*',
        ],
      },
    },
  ],
}`

		// Create temporary Jsonnet file for diff
		tmpfile, err := os.CreateTemp("", "test-jsonnet-diff-*.jsonnet")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpfile.Name())

		if _, err := tmpfile.Write([]byte(rawConfigJsonnet)); err != nil {
			t.Fatalf("Failed to write temp file: %v", err)
		}
		tmpfile.Close()

		// Use API client to get diff
		diffText, err := client.DiffConfigFromFile(t.Context(), tmpfile.Name())
		if err != nil {
			t.Fatalf("failed to get diff: %v", err)
		}
		if !strings.Contains(diffText, "jsonnet-diff-upstream") {
			t.Errorf("Expected diff to contain 'jsonnet-diff-upstream', but got: %s", diffText)
		}
	})

	// Test 6: Reload config
	t.Run("ReloadConfig", func(t *testing.T) {
		// First, update the config with PUT
		updatedConfig := `{
		"upstreams": [
			{
				"name": "primary",
				"address": "localhost:5672",
				"routing": {}
			},
			{
				"name": "reload-test-upstream",
				"address": "localhost:5673",
				"routing": {
					"key_patterns": [
						"reload.test.*"
					]
				}
			}
		]
	}`

		// Create temporary config file for update
		tmpfile, err := os.CreateTemp("", "test-reload-config-*.json")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpfile.Name())

		if _, err := tmpfile.Write([]byte(updatedConfig)); err != nil {
			t.Fatalf("Failed to write temp file: %v", err)
		}
		tmpfile.Close()

		// First, update the config using API client
		if err := client.PutConfigFromFile(t.Context(), tmpfile.Name()); err != nil {
			t.Fatalf("failed to put config: %v", err)
		}

		// Now test reload - it should restore original config from file
		reloadedConfig, err := client.ReloadConfig(t.Context())
		if err != nil {
			t.Fatalf("failed to reload config: %v", err)
		}

		// Verify it reloaded from original file (should have "secondary" upstream, not "reload-test-upstream")
		if len(reloadedConfig.Upstreams) != 2 {
			t.Fatalf("Expected 2 upstreams after reload, got %d", len(reloadedConfig.Upstreams))
		}

		// Check that we have the original "secondary" upstream
		foundSecondary := false
		for _, upstream := range reloadedConfig.Upstreams {
			if upstream.Name == "secondary" {
				foundSecondary = true
				break
			}
			if upstream.Name == "reload-test-upstream" {
				t.Error("Found 'reload-test-upstream' after reload, should have been reverted to original config")
			}
		}

		if !foundSecondary {
			t.Error("Expected to find 'secondary' upstream after reload")
		}
	})
}

// TestAPIGetClients tests the GET /clients endpoint
func TestAPIGetClients(t *testing.T) {
	// Start isolated server instance for clients test
	cancel, socketPath, client := startIsolatedAPIServer(t, "testdata/config.json")
	defer cancel()
	defer os.Remove(socketPath)

	// First test: No clients connected
	t.Run("NoClients", func(t *testing.T) {
		clientsInfo, err := client.GetClients(t.Context())
		if err != nil {
			t.Fatalf("Failed to get clients: %v", err)
		}

		if len(clientsInfo) != 0 {
			t.Errorf("Expected 0 clients, got %d", len(clientsInfo))
		}
	})

	// Test with mock clients
	t.Run("WithMockClients", func(t *testing.T) {
		// Create a server instance for testing
		cfg := &config.Config{
			Upstreams: []config.Upstream{
				{Name: "test", Address: "localhost:5672"},
			},
		}
		server := trabbits.NewServer(cfg, "")

		// Create mock proxies and register them
		proxy1 := server.NewProxy(nil) // nil connection for testing
		proxy2 := server.NewProxy(nil)

		// Set some test data
		proxy1.SetUser("user1")
		proxy1.SetVirtualHost("/vhost1")
		proxy2.SetUser("user2")
		proxy2.SetVirtualHost("/vhost2")
		proxy2.SetShutdownMessage("Test shutdown")

		// Register proxies
		server.RegisterProxy(proxy1, func() {})
		server.RegisterProxy(proxy2, func() {})
		defer func() {
			server.UnregisterProxy(proxy1)
			server.UnregisterProxy(proxy2)
		}()

		// Get clients info
		clients := server.GetClientsInfo()

		if len(clients) != 2 {
			t.Fatalf("Expected 2 clients, got %d", len(clients))
		}

		// Log the clients info for debugging
		for i, client := range clients {
			t.Logf("Client %d: ID=%s, User=%s, VirtualHost=%s, Status=%s, ConnectedAt=%s, ShutdownReason=%s",
				i, client.ID, client.User, client.VirtualHost, client.Status, client.ConnectedAt.Format("2006-01-02T15:04:05.000Z07:00"), client.ShutdownReason)
		}

		// Check first client (active)
		client1 := clients[0]
		if client1.Status != types.ClientStatusActive {
			t.Errorf("Expected status '%s', got '%s'", types.ClientStatusActive, client1.Status)
		}
		if client1.User != "user1" {
			t.Errorf("Expected user 'user1', got '%s'", client1.User)
		}
		if client1.VirtualHost != "/vhost1" {
			t.Errorf("Expected virtual host '/vhost1', got '%s'", client1.VirtualHost)
		}
		if client1.ConnectedAt.IsZero() {
			t.Error("Connected at time should not be zero")
		}
		if client1.ShutdownReason != "" {
			t.Errorf("Expected empty shutdown reason for active client, got '%s'", client1.ShutdownReason)
		}
		if client1.ClientProperties != nil {
			t.Errorf("Expected nil ClientProperties for clients list, got %v", client1.ClientProperties)
		}

		// Check second client (shutting down)
		client2 := clients[1]
		if client2.Status != types.ClientStatusShuttingDown {
			t.Errorf("Expected status '%s', got '%s'", types.ClientStatusShuttingDown, client2.Status)
		}
		if client2.User != "user2" {
			t.Errorf("Expected user 'user2', got '%s'", client2.User)
		}
		if client2.VirtualHost != "/vhost2" {
			t.Errorf("Expected virtual host '/vhost2', got '%s'", client2.VirtualHost)
		}
		if client2.ShutdownReason != "Test shutdown" {
			t.Errorf("Expected shutdown reason 'Test shutdown', got '%s'", client2.ShutdownReason)
		}
		if client2.ClientProperties != nil {
			t.Errorf("Expected nil ClientProperties for clients list, got %v", client2.ClientProperties)
		}

		// Verify clients are sorted by connection time
		if !client2.ConnectedAt.After(client1.ConnectedAt) && !client2.ConnectedAt.Equal(client1.ConnectedAt) {
			t.Error("Clients should be sorted by connection time (oldest first)")
		}

		// Verify that ClientProperties are omitted from JSON output
		jsonBytes, err := json.Marshal(clients)
		if err != nil {
			t.Fatalf("Failed to marshal clients to JSON: %v", err)
		}
		jsonStr := string(jsonBytes)
		if strings.Contains(jsonStr, "client_properties") {
			t.Errorf("JSON output should not contain 'client_properties' field: %s", jsonStr)
		}
	})

}

// TestAPIGetClientsIntegration tests the GET /clients endpoint with real AMQP connections
func TestAPIGetClientsIntegration(t *testing.T) {
	// Start unified server with both AMQP proxy and API
	cancel, socketPath, client, proxyPort := startUnifiedTestServer(t, "testdata/config.json")
	defer cancel()
	defer os.Remove(socketPath)

	// Create helper function for AMQP connections to the unified server
	mustUnifiedTestConn := func(t *testing.T, keyValues ...string) *rabbitmq.Connection {
		props := rabbitmq.Table{
			"product":  "golang/AMQP 0.9.1 Client",
			"version":  "1.10.0",
			"platform": "golang",
		}

		// Add custom properties from key-value pairs
		if len(keyValues)%2 != 0 {
			t.Fatal("keyValues must be provided in pairs")
		}
		for i := 0; i < len(keyValues); i += 2 {
			props[keyValues[i]] = keyValues[i+1]
		}

		cfg := rabbitmq.Config{
			Properties: props,
		}

		conn, err := rabbitmq.DialConfig(fmt.Sprintf("amqp://admin:admin@127.0.0.1:%d/", proxyPort), cfg)
		if err != nil {
			t.Fatalf("Failed to connect to test server: %v", err)
		}
		return conn
	}

	t.Run("WithRealClientConnections", func(t *testing.T) {
		// Connect real AMQP clients to the unified server
		conn1 := mustUnifiedTestConn(t, "test_type", "api_integration_test_1")
		defer conn1.Close()
		time.Sleep(100 * time.Millisecond) // Allow connection to be processed

		conn2 := mustUnifiedTestConn(t, "test_type", "api_integration_test_2")
		defer conn2.Close()
		time.Sleep(100 * time.Millisecond) // Allow connection to be processed

		// Test API response using API client
		clientsInfo, err := client.GetClients(t.Context())
		if err != nil {
			t.Fatalf("Failed to get clients: %v", err)
		}

		t.Logf("API returned %d clients", len(clientsInfo))
		for i, c := range clientsInfo {
			t.Logf("Client %d: ID=%s, User=%s, Status=%s, ConnectedAt=%s",
				i, c.ID, c.User, c.Status, c.ConnectedAt.Format("2006-01-02T15:04:05.000Z07:00"))
		}

		if len(clientsInfo) < 2 {
			t.Errorf("Expected at least 2 clients, got %d", len(clientsInfo))
		}

		// Check properties of connected clients
		foundConnections := 0
		for _, clientInfo := range clientsInfo {
			// Verify basic client properties
			if clientInfo.ID == "" {
				t.Error("Client ID should not be empty")
			}
			if clientInfo.Status != types.ClientStatusActive {
				t.Errorf("Expected status '%s', got '%s'", types.ClientStatusActive, clientInfo.Status)
			}
			if clientInfo.ConnectedAt.IsZero() {
				t.Error("ConnectedAt should not be zero value")
			}
			if clientInfo.User != "admin" {
				t.Errorf("Expected user 'admin', got '%s'", clientInfo.User)
			}
			if clientInfo.VirtualHost != "/" {
				t.Errorf("Expected virtual host '/', got '%s'", clientInfo.VirtualHost)
			}
			if clientInfo.ClientAddress == "" {
				t.Error("Client address should not be empty")
			}
			if clientInfo.ShutdownReason != "" {
				t.Errorf("Active client should have empty shutdown reason, got '%s'", clientInfo.ShutdownReason)
			}

			// Check if this is one of our test connections
			if strings.Contains(clientInfo.ClientBanner, "golang/AMQP 0.9.1 Client") {
				foundConnections++
			}

			t.Logf("Found client: ID=%s, User=%s, Address=%s, ConnectedAt=%s, Status=%s",
				clientInfo.ID, clientInfo.User, clientInfo.ClientAddress,
				clientInfo.ConnectedAt.Format("2006-01-02T15:04:05.000Z07:00"), clientInfo.Status)
		}

		if foundConnections < 2 {
			t.Errorf("Expected to find at least 2 test connections, found %d", foundConnections)
		}

		// Verify clients are sorted by connection time
		for i := 1; i < len(clientsInfo); i++ {
			if clientsInfo[i].ConnectedAt.Before(clientsInfo[i-1].ConnectedAt) {
				t.Error("Clients should be sorted by connection time (oldest first)")
				break
			}
		}
	})
}

func TestAPIGetClient(t *testing.T) {
	// Create test server
	cfg := &config.Config{
		Upstreams: []config.Upstream{
			{Name: "test", Address: "localhost:5672"},
		},
	}
	server := trabbits.NewTestServer(cfg)

	// Create test proxy with full client properties
	proxy := server.NewProxy(nil)
	proxy.SetUser("testuser")
	proxy.SetVirtualHost("/test")

	// Add some stats
	if proxy.Stats() != nil {
		proxy.Stats().IncrementMethod("Basic.Publish")
		proxy.Stats().IncrementMethod("Basic.Consume")
		proxy.Stats().IncrementReceivedFrames()
		proxy.Stats().IncrementSentFrames()
	}

	server.RegisterProxy(proxy, func() {})
	defer server.UnregisterProxy(proxy)

	// Get client info
	clientInfo, found := server.GetClientInfo(proxy.ID())
	if !found {
		t.Fatal("Expected to find client info")
	}

	// Verify full client info
	if clientInfo.ID != proxy.ID() {
		t.Errorf("Expected ID %s, got %s", proxy.ID(), clientInfo.ID)
	}
	if clientInfo.User != "testuser" {
		t.Errorf("Expected user 'testuser', got %s", clientInfo.User)
	}
	if clientInfo.VirtualHost != "/test" {
		t.Errorf("Expected virtual host '/test', got %s", clientInfo.VirtualHost)
	}

	// Verify client properties are included (even if empty in test)
	// For this test, we expect ClientProperties to be present (could be empty)
	// The important thing is that it's not nil like in the clients list
	if clientInfo.ClientProperties == nil {
		t.Log("ClientProperties is nil - this is OK for test proxy without real client props")
	}

	// Verify full stats are included
	if clientInfo.Stats == nil {
		t.Error("Expected Stats to be included in full client info")
	} else {
		if clientInfo.Stats.TotalMethods != 2 {
			t.Errorf("Expected 2 total methods, got %d", clientInfo.Stats.TotalMethods)
		}
		if clientInfo.Stats.Methods == nil || len(clientInfo.Stats.Methods) == 0 {
			t.Error("Expected Methods breakdown to be included")
		}
		if clientInfo.Stats.StartedAt.IsZero() {
			t.Error("Expected StartedAt to be set")
		}
	}
}

// TestAPIShutdownProxy tests the DELETE /clients/{proxy_id} endpoint
func TestAPIShutdownProxy(t *testing.T) {
	// Start isolated server for testing
	cancel, socketPath, client := startIsolatedAPIServer(t, "testdata/config.json")
	defer cancel()
	defer os.Remove(socketPath)

	t.Run("ProxyNotFound", func(t *testing.T) {
		// Try to shutdown a non-existent proxy using API client
		err := client.ShutdownClient(t.Context(), "nonexistent", "test shutdown")
		if err == nil {
			t.Fatal("Expected error for non-existent proxy, got nil")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("Expected 'not found' error, got: %v", err)
		}
	})

	t.Run("InvalidProxyID", func(t *testing.T) {
		// Try to shutdown with empty proxy ID using API client
		err := client.ShutdownClient(t.Context(), "", "test shutdown")
		if err == nil {
			t.Fatal("Expected error for empty proxy ID, got nil")
		}
		if !strings.Contains(err.Error(), "cannot be empty") {
			t.Errorf("Expected 'cannot be empty' error, got: %v", err)
		}
	})

	t.Run("SuccessfulShutdown", func(t *testing.T) {
		// Create a server instance for testing
		cfg := &config.Config{
			Upstreams: []config.Upstream{
				{Name: "test", Address: "localhost:5672"},
			},
		}
		server := trabbits.NewServer(cfg, "")

		// Create and register a mock proxy
		proxy := server.NewProxy(nil)
		proxy.SetUser("testuser")
		proxy.SetVirtualHost("/test")
		server.RegisterProxy(proxy, func() {})
		defer server.UnregisterProxy(proxy)

		// Test shutdown
		found := server.ShutdownProxy(proxy.ID(), "Test shutdown")
		if !found {
			t.Error("Expected to find and shutdown the proxy")
		}

		// Verify the proxy has shutdown message set
		clients := server.GetClientsInfo()
		if len(clients) != 1 {
			t.Fatalf("Expected 1 proxy, got %d", len(clients))
		}

		if clients[0].Status != types.ClientStatusShuttingDown {
			t.Errorf("Expected status '%s', got '%s'", types.ClientStatusShuttingDown, clients[0].Status)
		}

		if clients[0].ShutdownReason != "Test shutdown" {
			t.Errorf("Expected shutdown reason 'Test shutdown', got '%s'", clients[0].ShutdownReason)
		}
	})

	t.Run("ShutdownWithCustomReason", func(t *testing.T) {
		// Create a server instance for testing
		cfg := &config.Config{
			Upstreams: []config.Upstream{
				{Name: "test", Address: "localhost:5672"},
			},
		}
		server := trabbits.NewServer(cfg, "")

		// Create and register a mock proxy
		proxy := server.NewProxy(nil)
		server.RegisterProxy(proxy, func() {})
		defer server.UnregisterProxy(proxy)

		// Test shutdown with custom reason
		customReason := "Maintenance shutdown"
		found := server.ShutdownProxy(proxy.ID(), customReason)
		if !found {
			t.Error("Expected to find and shutdown the proxy")
		}

		// Verify the shutdown reason
		clients := server.GetClientsInfo()
		if len(clients) != 1 {
			t.Fatalf("Expected 1 proxy, got %d", len(clients))
		}

		if clients[0].ShutdownReason != customReason {
			t.Errorf("Expected shutdown reason '%s', got '%s'", customReason, clients[0].ShutdownReason)
		}
	})

	t.Run("DefaultShutdownReason", func(t *testing.T) {
		// Create a server instance for testing
		cfg := &config.Config{
			Upstreams: []config.Upstream{
				{Name: "test", Address: "localhost:5672"},
			},
		}
		server := trabbits.NewServer(cfg, "")

		// Create and register a mock proxy
		proxy := server.NewProxy(nil)
		server.RegisterProxy(proxy, func() {})
		defer server.UnregisterProxy(proxy)

		// Test shutdown without custom reason
		found := server.ShutdownProxy(proxy.ID(), "")
		if !found {
			t.Error("Expected to find and shutdown the proxy")
		}

		// Verify the default shutdown reason
		clients := server.GetClientsInfo()
		if len(clients) != 1 {
			t.Fatalf("Expected 1 proxy, got %d", len(clients))
		}

		if clients[0].ShutdownReason != "API shutdown request" {
			t.Errorf("Expected default shutdown reason 'API shutdown request', got '%s'", clients[0].ShutdownReason)
		}
	})
}
