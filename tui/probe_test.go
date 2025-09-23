package tui_test

import (
	"context"
	"testing"
	"time"

	"github.com/fujiwara/trabbits/tui"
	"github.com/fujiwara/trabbits/types"
)

// Mock API client for testing
type mockAPIClient struct {
	clients       []types.ClientInfo
	clientDetails map[string]*types.FullClientInfo
	probeStreams  map[string]chan tui.ProbeLogEntry
}

func (m *mockAPIClient) GetClients(ctx context.Context) ([]types.ClientInfo, error) {
	return m.clients, nil
}

func (m *mockAPIClient) GetClientDetail(ctx context.Context, clientID string) (*types.FullClientInfo, error) {
	if detail, exists := m.clientDetails[clientID]; exists {
		return detail, nil
	}
	return nil, nil
}

func (m *mockAPIClient) ShutdownClient(ctx context.Context, clientID, reason string) error {
	return nil
}

func (m *mockAPIClient) StreamProbeLog(ctx context.Context, proxyID string) (<-chan tui.ProbeLogEntry, error) {
	ch := make(chan tui.ProbeLogEntry, 10)
	m.probeStreams[proxyID] = ch
	return ch, nil
}

func newMockAPIClient() *mockAPIClient {
	return &mockAPIClient{
		clients: []types.ClientInfo{
			{
				ID:            "test-client-1",
				User:          "guest",
				ClientAddress: "127.0.0.1:12345",
				Status:        "active",
			},
		},
		clientDetails: make(map[string]*types.FullClientInfo),
		probeStreams:  make(map[string]chan tui.ProbeLogEntry),
	}
}

func TestTUIModelCreation(t *testing.T) {
	ctx := context.Background()
	client := newMockAPIClient()

	model := tui.NewModel(ctx, client)

	if model == nil {
		t.Fatal("NewModel returned nil")
	}
}

func TestProbeLogEntry(t *testing.T) {
	entry := tui.ProbeLogEntry{
		Timestamp: time.Now(),
		Message:   "test message",
		Attrs: map[string]any{
			"key1": "value1",
			"key2": 42,
		},
	}

	if entry.Message != "test message" {
		t.Errorf("expected message 'test message', got %s", entry.Message)
	}

	if entry.Attrs["key1"] != "value1" {
		t.Errorf("expected key1='value1', got %v", entry.Attrs["key1"])
	}

	if entry.Attrs["key2"] != 42 {
		t.Errorf("expected key2=42, got %v", entry.Attrs["key2"])
	}
}

func TestMockAPIClientProbeStream(t *testing.T) {
	ctx := context.Background()
	client := newMockAPIClient()

	// Test streaming
	logChan, err := client.StreamProbeLog(ctx, "test-client-1")
	if err != nil {
		t.Fatalf("StreamProbeLog failed: %v", err)
	}

	// Send a test log
	testLog := tui.ProbeLogEntry{
		Timestamp: time.Now(),
		Message:   "test probe log",
		Attrs:     map[string]any{"test": true},
	}

	client.probeStreams["test-client-1"] <- testLog

	// Receive the log
	select {
	case received := <-logChan:
		if received.Message != testLog.Message {
			t.Errorf("expected message %s, got %s", testLog.Message, received.Message)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for probe log")
	}
}
