package trabbits_test

import (
	"bytes"
	"context"
	"crypto/rand"
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
			return net.Dial("unix", testAPISock)
		},
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   30 * time.Second,
	}
	return client
}

func TestAPIGetPutGetConfig(t *testing.T) {
	endpoint := "http://localhost/config"
	client := newUnixSockHTTPClient(testAPISock)
	var testConfig trabbits.Config
	t.Run("GET returns 200 OK current config", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, endpoint, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("failed to send request: %v", err)
		}
		if code := resp.StatusCode; code != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", code, http.StatusOK)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("unexpected Content-Type: got %v want %v", ct, "application/json")
		}
		if err := json.NewDecoder(resp.Body).Decode(&testConfig); err != nil {
			t.Fatalf("failed to decode JSON: %v", err)
		}
	})

	if len(testConfig.Upstreams) != 2 {
		t.Fatalf("unexpected number of upstreams: got %v want %v", len(testConfig.Upstreams), 2)
	}
	// update config
	testConfig.Upstreams[1].Routing.KeyPatterns = append(
		testConfig.Upstreams[1].Routing.KeyPatterns, rand.Text(),
	)

	t.Run("PUT returns 200 OK", func(t *testing.T) {
		b := new(bytes.Buffer)
		json.NewEncoder(b).Encode(testConfig)
		req, _ := http.NewRequest(http.MethodPut, endpoint, io.NopCloser(b))
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("failed to send request: %v", err)
		}
		if code := resp.StatusCode; code != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", code, http.StatusOK)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("unexpected Content-Type: got %v want %v", ct, "application/json")
		}
	})

	t.Run("GET returns 200 OK with updated config", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, endpoint, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("failed to send request: %v", err)
		}
		if code := resp.StatusCode; code != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", code, http.StatusOK)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("unexpected Content-Type: got %v want %v", ct, "application/json")
		}
		var respondConfig trabbits.Config
		if err := json.NewDecoder(resp.Body).Decode(&respondConfig); err != nil {
			t.Fatalf("failed to decode JSON: %v", err)
		}
		if diff := cmp.Diff(testConfig, respondConfig); diff != "" {
			t.Errorf("unexpected response: %s", diff)
		}
	})
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

func TestAPIPutRawJSONConfig(t *testing.T) {
	endpoint := "http://localhost/config"
	client := newUnixSockHTTPClient(testAPISock)

	// Test raw JSON config with environment variables
	t.Setenv("TEST_API_USERNAME", "apiuser")
	t.Setenv("TEST_API_PASSWORD", "apipass")

	// Raw JSON config content (as it would be in a file)
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

	// Send raw JSON content (simulating what the new putConfigFromFile does)
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

	// Verify the config was properly processed on server side
	var respondConfig trabbits.Config
	if err := json.NewDecoder(resp.Body).Decode(&respondConfig); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	// Check that environment variables were expanded on server side
	if len(respondConfig.Upstreams) != 2 {
		t.Fatalf("Expected 2 upstreams, got %d", len(respondConfig.Upstreams))
	}

	testUpstream := respondConfig.Upstreams[1]
	if testUpstream.HealthCheck == nil {
		t.Fatal("Expected health check configuration, got nil")
	}

	// Environment variables should be expanded on server side
	if testUpstream.HealthCheck.Username != "apiuser" {
		t.Errorf("Expected username 'apiuser', got '%s'", testUpstream.HealthCheck.Username)
	}

	// Password should be masked in response (but internally correct)
	// The actual password is set correctly on server side but masked in JSON response
}

func TestAPIPutJsonnetConfig(t *testing.T) {
	endpoint := "http://localhost/config"
	client := newUnixSockHTTPClient(testAPISock)

	// Test Jsonnet config
	t.Setenv("TEST_JSONNET_USERNAME", "jsonnetuser")
	t.Setenv("TEST_JSONNET_PASSWORD", "jsonnetpass")

	// Raw Jsonnet config content
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

	// Create temporary Jsonnet file to simulate file-based config
	tmpfile, err := os.CreateTemp("", "test-api-config-*.jsonnet")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(rawConfigJsonnet)); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpfile.Close()

	// Read the file content back (simulating what putConfigFromFile does)
	data, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to read temp file: %v", err)
	}

	// Send raw Jsonnet content with appropriate Content-Type
	req, _ := http.NewRequest(http.MethodPut, endpoint, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/jsonnet")

	// Debug: check what we're sending
	t.Logf("Sending request with Content-Type: %s", req.Header.Get("Content-Type"))
	t.Logf("Request body length: %d", len(data))

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if code := resp.StatusCode; code != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", code, http.StatusOK)
	}

	// Verify the Jsonnet config was properly processed on server side
	var respondConfig trabbits.Config
	if err := json.NewDecoder(resp.Body).Decode(&respondConfig); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	// Check that Jsonnet was evaluated and environment variables were accessed
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

	// Environment variables should be accessed through std.native('env') on server side
	if jsonnetUpstream.HealthCheck.Username != "jsonnetuser" {
		t.Errorf("Expected username 'jsonnetuser', got '%s'", jsonnetUpstream.HealthCheck.Username)
	}
}
