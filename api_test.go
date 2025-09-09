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

	cli := &trabbits.CLI{
		APISocket: socketPath,
		Config:    configFile,
	}

	go func() {
		_, err := trabbits.RunAPIServer(ctx, cli)
		if err != nil && ctx.Err() == nil {
			t.Errorf("API server failed: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	client := newUnixSockHTTPClient(socketPath)
	return cancel, socketPath, client
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
		var testConfig trabbits.Config
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
		var updatedConfig trabbits.Config
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
					"interval": "30s",
					"timeout": "5s",
					"unhealthy_threshold": 3,
					"recovery_interval": "60s",
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

		var respondConfig trabbits.Config
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
        interval: '45s',
        timeout: '10s',
        unhealthy_threshold: 2,
        recovery_interval: '90s',
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

		var respondConfig trabbits.Config
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
					"interval": "45s",
					"timeout": "10s",
					"unhealthy_threshold": 2,
					"recovery_interval": "90s",
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
        interval: '60s',
        timeout: '15s',
        unhealthy_threshold: 4,
        recovery_interval: '120s',
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
}
