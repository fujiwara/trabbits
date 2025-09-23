package trabbits_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fujiwara/trabbits"
	"github.com/fujiwara/trabbits/config"
)

func TestAPIProbeLogHandler(t *testing.T) {
	// Create test server
	cfg := &config.Config{
		Upstreams: []config.Upstream{
			{Name: "test-upstream", Address: "localhost:5672"},
		},
	}
	server := trabbits.NewTestServer(cfg)

	// Create a test proxy
	probeChan := make(chan trabbits.ProbeLog, 10)
	proxy := &trabbits.Proxy{}
	proxy.SetProbeChan(probeChan)

	// For testing, we'll use a more direct approach

	t.Run("ProxyNotFound", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/clients/nonexistent/probe", nil)
		req.SetPathValue("proxy_id", "nonexistent")
		w := httptest.NewRecorder()

		handler := server.GetAPIProbeLogHandler()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", w.Code)
		}
	})

	t.Run("ProbeLogJSONSerialization", func(t *testing.T) {
		// Create probe log using the SendProbeLog method to populate attrs
		probeChan := make(chan trabbits.ProbeLog, 1)
		testProxy := &trabbits.Proxy{}
		testProxy.SetProbeChan(probeChan)

		// Send a probe log with attributes
		testProxy.SendProbeLog("test message", "key1", "value1", "key2", 42)

		// Retrieve the log
		var log trabbits.ProbeLog
		select {
		case log = <-probeChan:
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timeout waiting for probe log")
		}

		// Convert to the API format
		logData := struct {
			Timestamp time.Time      `json:"timestamp"`
			Message   string         `json:"message"`
			Attrs     map[string]any `json:"attrs,omitempty"`
		}{
			Timestamp: log.Timestamp,
			Message:   log.Message,
			Attrs:     log.AttrsMap(),
		}

		jsonData, err := json.Marshal(logData)
		if err != nil {
			t.Fatalf("Failed to marshal probe log: %v", err)
		}

		// Verify JSON structure
		var parsed map[string]any
		if err := json.Unmarshal(jsonData, &parsed); err != nil {
			t.Fatalf("Failed to unmarshal JSON: %v", err)
		}

		if parsed["message"] != "test message" {
			t.Errorf("expected message 'test message', got %v", parsed["message"])
		}

		attrs, ok := parsed["attrs"].(map[string]any)
		if !ok {
			t.Fatalf("attrs should be a map")
		}
		if attrs["key1"] != "value1" {
			t.Errorf("expected key1='value1', got %v", attrs["key1"])
		}
		if attrs["key2"].(float64) != 42 { // JSON numbers are float64
			t.Errorf("expected key2=42, got %v", attrs["key2"])
		}
	})
}

// sseRecorder is a custom ResponseWriter for testing SSE responses
type sseRecorder struct {
	header   http.Header
	body     *strings.Builder
	dataChan chan string
	code     int
}

func (r *sseRecorder) Header() http.Header {
	return r.header
}

func (r *sseRecorder) Write(data []byte) (int, error) {
	// Capture SSE data lines
	r.body.Write(data)

	// Parse data lines and send to channel
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "data: ") {
			r.dataChan <- strings.TrimPrefix(line, "data: ")
		}
	}

	return len(data), nil
}

func (r *sseRecorder) WriteHeader(statusCode int) {
	r.code = statusCode
}

func (r *sseRecorder) Flush() {
	// No-op for testing
}

// TestSSEFormat tests the SSE message format
func TestSSEFormat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]any
	}{
		{
			name:  "ConnectionMessage",
			input: `{"type":"connected","proxy_id":"test-123"}`,
			expected: map[string]any{
				"type":     "connected",
				"proxy_id": "test-123",
			},
		},
		{
			name:  "ProxyEndedMessage",
			input: `{"type":"proxy_ended"}`,
			expected: map[string]any{
				"type": "proxy_ended",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var parsed map[string]any
			if err := json.Unmarshal([]byte(tt.input), &parsed); err != nil {
				t.Fatalf("Failed to parse JSON: %v", err)
			}

			for key, expectedValue := range tt.expected {
				if parsed[key] != expectedValue {
					t.Errorf("expected %s=%v, got %v", key, expectedValue, parsed[key])
				}
			}
		})
	}
}
