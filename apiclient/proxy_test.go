package apiclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
			err := client.ShutdownClient(context.Background(), tt.clientID, tt.reason)

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
	err := client.ShutdownClient(context.Background(), "test-client", "test reason")

	// Verify no error
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify DELETE method was used
	if methodCalled != "DELETE" {
		t.Errorf("expected DELETE method to be called, got %s", methodCalled)
	}
}
