package apiclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fujiwara/trabbits/apiclient"
)

func TestShutdownClient(t *testing.T) {
	tests := []struct {
		name           string
		clientID       string
		reason         string
		serverHandler  http.HandlerFunc
		expectError    bool
		expectedMethod string
		expectedPath   string
	}{
		{
			name:           "successful shutdown with DELETE method",
			clientID:       "test-client-123",
			reason:         "test shutdown",
			expectedMethod: "DELETE",
			expectedPath:   "/clients/test-client-123",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				// Verify the correct method is used
				if r.Method != "DELETE" {
					t.Errorf("expected DELETE method, got %s", r.Method)
					http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
					return
				}
				// Verify the path
				if r.URL.Path != "/clients/test-client-123" {
					t.Errorf("expected path /clients/test-client-123, got %s", r.URL.Path)
					http.Error(w, "Not Found", http.StatusNotFound)
					return
				}
				// Verify reason is passed as query parameter
				reason := r.URL.Query().Get("reason")
				if reason != "test shutdown" {
					t.Errorf("expected reason 'test shutdown', got %s", reason)
				}

				// Return success response
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]string{
					"status":   "shutdown_initiated",
					"proxy_id": "test-client-123",
					"reason":   reason,
				})
			},
			expectError: false,
		},
		{
			name:           "client not found",
			clientID:       "non-existent-client",
			reason:         "",
			expectedMethod: "DELETE",
			expectedPath:   "/clients/non-existent-client",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "Proxy not found", http.StatusNotFound)
			},
			expectError: true,
		},
		{
			name:           "empty client ID",
			clientID:       "",
			reason:         "test",
			expectedMethod: "DELETE",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				// This should not be called since validation happens client-side
				t.Error("handler should not be called for empty client ID")
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(tt.serverHandler)
			defer server.Close()

			// Create client with test server URL
			client := apiclient.New(server.URL)

			// Call ShutdownClient
			err := client.ShutdownClient(t.Context(), tt.clientID, tt.reason)

			// Check error expectation
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestShutdownClient_CorrectMethod verifies that DELETE method is used correctly
func TestShutdownClient_CorrectMethod(t *testing.T) {
	// This test verifies that the client correctly uses DELETE method
	methodCalled := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methodCalled = r.Method
		// The API server expects DELETE method
		if r.Method != "DELETE" {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":   "shutdown_initiated",
			"proxy_id": "test-client",
		})
	}))
	defer server.Close()

	client := apiclient.New(server.URL)

	// This should succeed since we're using DELETE method
	err := client.ShutdownClient(t.Context(), "test-client", "test reason")

	// Verify no error
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify DELETE method was used
	if methodCalled != "DELETE" {
		t.Errorf("expected DELETE method to be called, got %s", methodCalled)
	}
}

func TestStreamServerLogs(t *testing.T) {
	// Create a test server that sends SSE events
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify correct endpoint
		if r.URL.Path != "/logs" {
			t.Errorf("expected path /logs, got %s", r.URL.Path)
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}

		// Set SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// Send test log entries
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		// Send a few test logs
		logs := []map[string]any{
			{
				"timestamp": "2025-01-20T10:00:00Z",
				"message":   "Server started",
				"attrs": map[string]any{
					"level": "INFO",
					"port":  6672,
				},
			},
			{
				"timestamp": "2025-01-20T10:00:01Z",
				"message":   "Client connected",
				"attrs": map[string]any{
					"level":     "INFO",
					"client_id": "test-123",
					"address":   "127.0.0.1:54321",
				},
			},
		}

		for _, log := range logs {
			data, _ := json.Marshal(log)
			w.Write([]byte("data: "))
			w.Write(data)
			w.Write([]byte("\n\n"))
			flusher.Flush()
		}
	}))
	defer server.Close()

	client := apiclient.New(server.URL)

	// Stream logs
	ctx := t.Context()
	logChan, err := client.StreamServerLogs(ctx)
	if err != nil {
		t.Fatalf("StreamServerLogs failed: %v", err)
	}

	// Receive first log
	select {
	case log := <-logChan:
		if log.Message != "Server started" {
			t.Errorf("expected message 'Server started', got %q", log.Message)
		}
		if log.Attrs["level"] != "INFO" {
			t.Errorf("expected level INFO, got %v", log.Attrs["level"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for first log")
	}

	// Receive second log
	select {
	case log := <-logChan:
		if log.Message != "Client connected" {
			t.Errorf("expected message 'Client connected', got %q", log.Message)
		}
		if log.Attrs["client_id"] != "test-123" {
			t.Errorf("expected client_id test-123, got %v", log.Attrs["client_id"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for second log")
	}
}

func TestStreamProbeLogEntries(t *testing.T) {
	// Create a test server that sends SSE events
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify correct endpoint
		if r.URL.Path != "/clients/test-proxy-123/probe" {
			t.Errorf("expected path /clients/test-proxy-123/probe, got %s", r.URL.Path)
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}

		// Set SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		// Send connection event
		connEvent := map[string]any{
			"type":     "connected",
			"proxy_id": "test-proxy-123",
		}
		data, _ := json.Marshal(connEvent)
		w.Write([]byte("data: "))
		w.Write(data)
		w.Write([]byte("\n\n"))
		flusher.Flush()

		// Send probe log entries
		logs := []map[string]any{
			{
				"timestamp": "2025-01-20T10:00:00Z",
				"message":   "Channel.Open",
				"attrs": map[string]any{
					"channel": 1,
				},
			},
			{
				"timestamp": "2025-01-20T10:00:01Z",
				"message":   "Queue.Declare",
				"attrs": map[string]any{
					"queue":   "test.queue",
					"durable": true,
				},
			},
		}

		for _, log := range logs {
			data, _ := json.Marshal(log)
			w.Write([]byte("data: "))
			w.Write(data)
			w.Write([]byte("\n\n"))
			flusher.Flush()
		}

		// Send end event
		endEvent := map[string]any{"type": "proxy_ended"}
		data, _ = json.Marshal(endEvent)
		w.Write([]byte("data: "))
		w.Write(data)
		w.Write([]byte("\n\n"))
		flusher.Flush()
	}))
	defer server.Close()

	client := apiclient.New(server.URL)

	// Stream probe logs
	ctx := t.Context()
	logChan, err := client.StreamProbeLogEntries(ctx, "test-proxy-123")
	if err != nil {
		t.Fatalf("StreamProbeLogEntries failed: %v", err)
	}

	// Receive first log
	select {
	case log := <-logChan:
		if log.Message != "Channel.Open" {
			t.Errorf("expected message 'Channel.Open', got %q", log.Message)
		}
		if log.Attrs["channel"] != float64(1) { // JSON unmarshals numbers as float64
			t.Errorf("expected channel 1, got %v", log.Attrs["channel"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for first log")
	}

	// Receive second log
	select {
	case log := <-logChan:
		if log.Message != "Queue.Declare" {
			t.Errorf("expected message 'Queue.Declare', got %q", log.Message)
		}
		if log.Attrs["queue"] != "test.queue" {
			t.Errorf("expected queue test.queue, got %v", log.Attrs["queue"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for second log")
	}

	// Channel should be closed after proxy_ended event
	select {
	case _, ok := <-logChan:
		if ok {
			t.Error("expected channel to be closed after proxy_ended event")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for channel close")
	}
}

func TestStreamProbeLogEntries_ContextCancellation(t *testing.T) {
	// Create a server that streams indefinitely
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		// Send connection event
		connEvent := map[string]any{"type": "connected", "proxy_id": "test-proxy"}
		data, _ := json.Marshal(connEvent)
		w.Write([]byte("data: "))
		w.Write(data)
		w.Write([]byte("\n\n"))
		flusher.Flush()

		// Keep connection open
		<-r.Context().Done()
	}))
	defer server.Close()

	client := apiclient.New(server.URL)

	// Create cancellable context
	ctx, cancel := context.WithCancel(t.Context())

	logChan, err := client.StreamProbeLogEntries(ctx, "test-proxy")
	if err != nil {
		t.Fatalf("StreamProbeLogEntries failed: %v", err)
	}

	// Cancel context
	cancel()

	// Channel should be closed
	select {
	case _, ok := <-logChan:
		if ok {
			t.Error("expected channel to be closed after context cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for channel close")
	}
}
