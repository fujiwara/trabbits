package tui_test

import (
	"context"
	"testing"
	"time"

	"github.com/fujiwara/trabbits/apiclient"
	"github.com/fujiwara/trabbits/config"
	"github.com/fujiwara/trabbits/tui"
	"github.com/fujiwara/trabbits/types"
)

// Mock API client for testing
type mockAPIClient struct {
	clients       []types.ClientInfo
	clientDetails map[string]*types.FullClientInfo
	probeStreams  map[string]chan types.ProbeLogEntry
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

func (m *mockAPIClient) StreamProbeLogEntries(ctx context.Context, proxyID string) (<-chan types.ProbeLogEntry, error) {
	ch := make(chan types.ProbeLogEntry, 10)
	m.probeStreams[proxyID] = ch
	return ch, nil
}

// Implement remaining methods to satisfy apiclient.APIClient interface
func (m *mockAPIClient) GetConfig(ctx context.Context) (*config.Config, error) {
	return &config.Config{}, nil
}

func (m *mockAPIClient) PutConfigFromFile(ctx context.Context, configPath string) error {
	return nil
}

func (m *mockAPIClient) PutConfig(ctx context.Context, cfg *config.Config) error {
	return nil
}

func (m *mockAPIClient) DiffConfigFromFile(ctx context.Context, configPath string) (string, error) {
	return "", nil
}

func (m *mockAPIClient) ReloadConfig(ctx context.Context) (*config.Config, error) {
	return &config.Config{}, nil
}

func (m *mockAPIClient) StreamServerLogs(ctx context.Context) (<-chan types.ProbeLogEntry, error) {
	ch := make(chan types.ProbeLogEntry, 10)
	// Return empty channel for tests
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

// Ensure mockAPIClient implements apiclient.APIClient
var _ apiclient.APIClient = (*mockAPIClient)(nil)

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
		probeStreams:  make(map[string]chan types.ProbeLogEntry),
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
	entry := types.ProbeLogEntry{
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
	logChan, err := client.StreamProbeLogEntries(ctx, "test-client-1")
	if err != nil {
		t.Fatalf("StreamProbeLog failed: %v", err)
	}

	// Send a test log
	testLog := types.ProbeLogEntry{
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
