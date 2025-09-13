package trabbits_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fujiwara/trabbits"
	"github.com/fujiwara/trabbits/config"
	"github.com/google/go-cmp/cmp"
)

func newUnixSockHTTPClient(socketPath string) *http.Client {
	tr := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   30 * time.Second,
	}
	return client
}

// startIsolatedAPIServer starts a new API server instance for isolated testing
func startIsolatedAPIServer(t *testing.T, configFile string) (context.CancelFunc, string, *http.Client) {
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

	client := newUnixSockHTTPClient(socketPath)
	return cancel, socketPath, client
}

// waitForTestAPI waits for API socket to be available
func waitForTestAPI(t *testing.T, socketPath string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socketPath); err == nil {
			// Try to actually connect
			client := newUnixSockHTTPClient(socketPath)
			if resp, err := client.Get("http://unix/config"); err == nil {
				resp.Body.Close()
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("API socket not available after %v", timeout)
}

func TestAPIPutInvalidConfig(t *testing.T) {
	endpoint := "http://localhost/config"
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode("invalid")
	req, _ := http.NewRequest(http.MethodPut, endpoint, b)
	req.Header.Set("Content-Type", "application/json")

	client := newUnixSockHTTPClient(testAPISock)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to send request: %v", err)
	}
	if code := resp.StatusCode; code != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v", code, http.StatusBadRequest)
	}
}

// TestAPIConfigUpdateIsolated runs all config-update tests in isolation with a dedicated server
func TestAPIConfigUpdateIsolated(t *testing.T) {
	// Start isolated server instance for config update tests
	cancel, socketPath, client := startIsolatedAPIServer(t, "testdata/config.json")
	defer cancel()
	defer os.Remove(socketPath)

	endpoint := "http://localhost/config"

	// Test 1: Basic PUT/GET cycle
	t.Run("BasicPutGetConfig", func(t *testing.T) {
		var testConfig config.Config
		// GET current config
		req, _ := http.NewRequest(http.MethodGet, endpoint, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("failed to send request: %v", err)
		}
		if code := resp.StatusCode; code != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", code, http.StatusOK)
		}
		if err := json.NewDecoder(resp.Body).Decode(&testConfig); err != nil {
			t.Fatalf("failed to decode JSON: %v", err)
		}
		resp.Body.Close()

		// Update config
		if len(testConfig.Upstreams) >= 2 {
			testConfig.Upstreams[1].Routing.KeyPatterns = append(
				testConfig.Upstreams[1].Routing.KeyPatterns, "test.isolated.*",
			)
		}

		// PUT updated config
		b := new(bytes.Buffer)
		json.NewEncoder(b).Encode(testConfig)
		req, _ = http.NewRequest(http.MethodPut, endpoint, b)
		req.Header.Set("Content-Type", "application/json")
		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("failed to send request: %v", err)
		}
		if code := resp.StatusCode; code != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", code, http.StatusOK)
		}
		resp.Body.Close()

		// Verify updated config
		req, _ = http.NewRequest(http.MethodGet, endpoint, nil)
		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("failed to send request: %v", err)
		}
		var updatedConfig config.Config
		if err := json.NewDecoder(resp.Body).Decode(&updatedConfig); err != nil {
			t.Fatalf("failed to decode JSON: %v", err)
		}
		resp.Body.Close()
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

		req, _ := http.NewRequest(http.MethodPut, endpoint, strings.NewReader(rawConfigJSON))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if code := resp.StatusCode; code != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", code, http.StatusOK)
		}

		var respondConfig config.Config
		if err := json.NewDecoder(resp.Body).Decode(&respondConfig); err != nil {
			t.Fatalf("failed to decode JSON: %v", err)
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

		data, err := os.ReadFile(tmpfile.Name())
		if err != nil {
			t.Fatalf("Failed to read temp file: %v", err)
		}

		req, _ := http.NewRequest(http.MethodPut, endpoint, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/jsonnet")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if code := resp.StatusCode; code != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", code, http.StatusOK)
		}

		var respondConfig config.Config
		if err := json.NewDecoder(resp.Body).Decode(&respondConfig); err != nil {
			t.Fatalf("failed to decode JSON: %v", err)
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
		diffEndpoint := endpoint + "/diff"
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

		req, _ := http.NewRequest(http.MethodPost, diffEndpoint, strings.NewReader(rawConfigJSON))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if code := resp.StatusCode; code != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", code, http.StatusOK)
		}

		if ct := resp.Header.Get("Content-Type"); ct != "text/plain" {
			t.Errorf("unexpected Content-Type: got %v want %v", ct, "text/plain")
		}

		diffBody, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read diff response: %v", err)
		}

		diffText := string(diffBody)
		if !strings.Contains(diffText, "diff-test-upstream") {
			t.Errorf("Expected diff to contain 'diff-test-upstream', but got: %s", diffText)
		}
	})

	// Test 5: Diff with Jsonnet config
	t.Run("DiffJsonnetConfig", func(t *testing.T) {
		diffEndpoint := endpoint + "/diff"
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

		req, _ := http.NewRequest(http.MethodPost, diffEndpoint, strings.NewReader(rawConfigJsonnet))
		req.Header.Set("Content-Type", "application/jsonnet")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if code := resp.StatusCode; code != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", code, http.StatusOK)
		}

		if ct := resp.Header.Get("Content-Type"); ct != "text/plain" {
			t.Errorf("unexpected Content-Type: got %v want %v", ct, "text/plain")
		}

		diffBody, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read diff response: %v", err)
		}

		diffText := string(diffBody)
		if !strings.Contains(diffText, "jsonnet-diff-upstream") {
			t.Errorf("Expected diff to contain 'jsonnet-diff-upstream', but got: %s", diffText)
		}
	})

	// Test 6: Reload config
	t.Run("ReloadConfig", func(t *testing.T) {
		reloadEndpoint := endpoint + "/reload"

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

		req, _ := http.NewRequest(http.MethodPut, endpoint, strings.NewReader(updatedConfig))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("failed to send PUT request: %v", err)
		}
		resp.Body.Close()

		// Now test reload - it should restore original config from file
		req, _ = http.NewRequest(http.MethodPost, reloadEndpoint, nil)
		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("failed to send reload request: %v", err)
		}
		defer resp.Body.Close()

		if code := resp.StatusCode; code != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", code, http.StatusOK)
		}

		var reloadedConfig config.Config
		if err := json.NewDecoder(resp.Body).Decode(&reloadedConfig); err != nil {
			t.Fatalf("failed to decode JSON: %v", err)
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
