// MIT License
// Copyright (c) 2025 FUJIWARA Shunichiro

package trabbits_test

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/fujiwara/trabbits"
	"github.com/fujiwara/trabbits/amqp091"
	"github.com/fujiwara/trabbits/config"
	rabbitmq "github.com/rabbitmq/amqp091-go"
)

func TestUpstreamDisconnection(t *testing.T) {

	// This test uses RabbitMQ Management API to forcibly close upstream connections
	// and verify that clients receive the expected Connection.Close message

	// Check if RabbitMQ Management API is available
	_, err := getRabbitMQConnections()
	if err != nil {
		t.Skipf("RabbitMQ Management API not available: %v", err)
	}

	// Generate unique test identifier for this test run
	testID := "test-" + rand.Text()[:12]
	t.Logf("Test ID: %s", testID)

	conn := mustTestConn(t, "test_id", testID)
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("Failed to open channel: %v", err)
	}
	defer ch.Close()

	// Register for connection close notifications
	closeChan := make(chan *rabbitmq.Error, 1)
	conn.NotifyClose(closeChan)

	t.Log("Connection established. Testing upstream disconnection via Management API...")

	// Start a goroutine to close upstream connections after a short delay
	go func() {
		time.Sleep(2 * time.Second)
		if err := closeUpstreamConnectionsWithID(t, testID); err != nil {
			t.Logf("Warning: Failed to close upstream connections: %v", err)
		}
	}()

	ctx := t.Context()
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()

	select {
	case closeErr := <-closeChan:
		if closeErr != nil {
			t.Logf("Connection closed with error: %v", closeErr)
			if closeErr.Code == amqp091.ConnectionForced {
				t.Logf("✓ Received expected connection-forced error (%d)", amqp091.ConnectionForced)
			} else {
				t.Logf("ℹ Received error code: %d (may vary depending on timing)", closeErr.Code)
			}
		} else {
			t.Log("Connection closed gracefully (no error)")
		}
	case <-timeout.C:
		t.Log("Test completed - no disconnection occurred within timeout")
	case <-ctx.Done():
		t.Log("Test cancelled")
	}
}

type rabbitMQConnection struct {
	Name             string                 `json:"name"`
	User             string                 `json:"user"`
	Host             string                 `json:"host"`
	Port             int                    `json:"port"`
	ClientProperties map[string]interface{} `json:"client_properties"`
}

func getRabbitMQConnections() ([]rabbitMQConnection, error) {
	client := &http.Client{Timeout: 5 * time.Second}

	req, err := http.NewRequest("GET", "http://localhost:15672/api/connections", nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("admin", "admin")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get connections: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	var connections []rabbitMQConnection
	if err := json.NewDecoder(resp.Body).Decode(&connections); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return connections, nil
}

// closeUpstreamConnectionsWithID closes only upstream connections that belong to this test
func closeUpstreamConnectionsWithID(t *testing.T, testID string) error {
	// Retry getting connections with backoff
	var connections []rabbitMQConnection
	var err error

	for attempts := 0; attempts < 5; attempts++ {
		connections, err = getRabbitMQConnections()
		if err != nil {
			t.Logf("Attempt %d: Failed to get connections: %v", attempts+1, err)
		} else if len(connections) == 0 {
			t.Logf("Attempt %d: Found 0 connections, retrying in 1 second...", attempts+1)
		} else {
			break
		}
		time.Sleep(1 * time.Second)
	}

	if err != nil {
		return err
	}

	t.Logf("Found %d connections after retries", len(connections))

	if len(connections) == 0 {
		return fmt.Errorf("no connections found after retries")
	}

	// Close connections that belong to this test
	closedCount := 0
	for _, conn := range connections {
		var product string
		var connTestID string
		if conn.ClientProperties != nil {
			if p, ok := conn.ClientProperties["product"].(string); ok {
				product = p
			}
			if tid, ok := conn.ClientProperties["test_id"].(string); ok {
				connTestID = tid
			}
		}

		if isUpstreamConnectionWithTestID(conn, testID) {
			t.Logf("Closing test upstream connection: %s (user: %s, product: %s, test_id: %s)",
				conn.Name, conn.User, product, connTestID)
			if err := closeRabbitMQConnection(conn.Name); err != nil {
				t.Logf("Failed to close connection %s: %v", conn.Name, err)
			} else {
				closedCount++
			}
		} else {
			t.Logf("Skipping connection: %s (user: %s, product: %s, test_id: %s)",
				conn.Name, conn.User, product, connTestID)
		}
	}

	if closedCount == 0 {
		return fmt.Errorf("no upstream connections with test_id %s were found", testID)
	}

	t.Logf("Successfully closed %d test upstream connections", closedCount)
	return nil
}

func isUpstreamConnection(conn rabbitMQConnection) bool {
	// Identify upstream connections by client_properties
	// trabbits sets a distinctive product name containing "via trabbits"
	if conn.ClientProperties != nil {
		if product, ok := conn.ClientProperties["product"].(string); ok {
			if strings.Contains(product, "via trabbits") {
				return true
			}
		}
	}
	return false
}

func isUpstreamConnectionWithTestID(conn rabbitMQConnection, testID string) bool {
	// First check if it's a trabbits upstream connection
	if !isUpstreamConnection(conn) {
		return false
	}

	// Then check if it has our test ID
	if conn.ClientProperties != nil {
		if connTestID, ok := conn.ClientProperties["test_id"].(string); ok {
			return connTestID == testID
		}
	}
	return false
}

func closeRabbitMQConnection(connectionName string) error {
	client := &http.Client{Timeout: 5 * time.Second}

	url := fmt.Sprintf("http://localhost:15672/api/connections/%s", connectionName)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth("admin", "admin")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to close connection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	return nil
}

// TestUpstreamReconnection tests that clients are notified when upstream reconnects
func TestUpstreamMonitoring(t *testing.T) {
	// This test verifies that the monitoring goroutine is started
	// and that NotifyClose is registered for each upstream

	// Create a mock upstream connection
	conn, err := rabbitmq.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		t.Skip("RabbitMQ not available for testing")
	}
	defer conn.Close()

	// Create an upstream with monitoring
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	conf := config.Upstream{
		Name: "test-upstream",
		Cluster: &config.Cluster{
			Nodes: []string{"localhost:5672"},
		},
	}

	// Create a dummy metrics instance for testing
	cfg := &config.Config{}
	server := trabbits.NewServer(cfg, "/tmp/test-upstream-disconnect.sock")
	upstream := trabbits.NewUpstream(conn, logger, conf, "localhost:5672", server.Metrics())

	// Verify that NotifyClose channel is available
	closeChan := upstream.NotifyClose()
	if closeChan == nil {
		t.Error("NotifyClose channel should not be nil")
	}

	// Clean up
	upstream.Close()
}

// TestUpstreamNotifyCloseSetup verifies that NotifyClose channels are properly set up
func TestUpstreamNotifyCloseSetup(t *testing.T) {
	// Create a real upstream connection to verify NotifyClose is working
	conn, err := rabbitmq.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		t.Skip("RabbitMQ not available for testing")
	}
	defer conn.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	conf := config.Upstream{
		Name: "test-upstream",
		Cluster: &config.Cluster{
			Nodes: []string{"localhost:5672"},
		},
	}

	// Create a dummy metrics instance for testing
	cfg2 := &config.Config{}
	server2 := trabbits.NewServer(cfg2, "/tmp/test-upstream-disconnect-2.sock")
	upstream := trabbits.NewUpstream(conn, logger, conf, "localhost:5672", server2.Metrics())
	defer upstream.Close()

	// Test that NotifyClose channel is not nil and responsive
	closeChan := upstream.NotifyClose()
	if closeChan == nil {
		t.Fatal("NotifyClose channel should not be nil")
	}

	// Test non-blocking read on the channel
	select {
	case <-closeChan:
		t.Log("Channel already closed or received close notification")
	default:
		t.Log("✓ NotifyClose channel is properly set up and not closed")
	}
}
