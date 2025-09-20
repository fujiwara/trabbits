package trabbits_test

import (
	"testing"
	"time"

	"github.com/fujiwara/trabbits"
	"github.com/fujiwara/trabbits/config"
)

func TestProxyStats(t *testing.T) {
	// Create a new stats instance
	stats := trabbits.NewProxyStats()

	// Test initial state
	if stats.GetTotalMethods() != 0 {
		t.Errorf("Expected initial total methods to be 0, got %d", stats.GetTotalMethods())
	}
	if stats.GetTotalFrames() != 0 {
		t.Errorf("Expected initial total frames to be 0, got %d", stats.GetTotalFrames())
	}

	// Test incrementing methods
	stats.IncrementMethod("Basic.Publish")
	stats.IncrementMethod("Basic.Publish")
	stats.IncrementMethod("Queue.Declare")

	if stats.GetMethodCount("Basic.Publish") != 2 {
		t.Errorf("Expected Basic.Publish count to be 2, got %d", stats.GetMethodCount("Basic.Publish"))
	}
	if stats.GetMethodCount("Queue.Declare") != 1 {
		t.Errorf("Expected Queue.Declare count to be 1, got %d", stats.GetMethodCount("Queue.Declare"))
	}
	if stats.GetTotalMethods() != 3 {
		t.Errorf("Expected total methods to be 3, got %d", stats.GetTotalMethods())
	}

	// Test incrementing frames
	stats.IncrementReceivedFrames()
	stats.IncrementReceivedFrames()
	stats.IncrementReceivedFrames()
	stats.IncrementSentFrames()
	stats.IncrementSentFrames()

	if stats.GetReceivedFrames() != 3 {
		t.Errorf("Expected received frames to be 3, got %d", stats.GetReceivedFrames())
	}
	if stats.GetSentFrames() != 2 {
		t.Errorf("Expected sent frames to be 2, got %d", stats.GetSentFrames())
	}
	if stats.GetTotalFrames() != 5 {
		t.Errorf("Expected total frames to be 5, got %d", stats.GetTotalFrames())
	}

	// Test snapshot
	snapshot := stats.Snapshot()
	if snapshot.TotalMethods != 3 {
		t.Errorf("Expected snapshot total methods to be 3, got %d", snapshot.TotalMethods)
	}
	if snapshot.ReceivedFrames != 3 {
		t.Errorf("Expected snapshot received frames to be 3, got %d", snapshot.ReceivedFrames)
	}
	if snapshot.SentFrames != 2 {
		t.Errorf("Expected snapshot sent frames to be 2, got %d", snapshot.SentFrames)
	}
	if snapshot.TotalFrames != 5 {
		t.Errorf("Expected snapshot total frames to be 5, got %d", snapshot.TotalFrames)
	}
	if len(snapshot.Methods) != 2 {
		t.Errorf("Expected 2 methods in snapshot, got %d", len(snapshot.Methods))
	}
	if snapshot.Methods["Basic.Publish"] != 2 {
		t.Errorf("Expected Basic.Publish count in snapshot to be 2, got %d", snapshot.Methods["Basic.Publish"])
	}

	// Test duration
	time.Sleep(10 * time.Millisecond)
	snapshot2 := stats.Snapshot()
	if snapshot2.Duration == "" {
		t.Error("Expected duration to be non-empty")
	}
}

func TestProxyStatsAPI(t *testing.T) {
	// Test the basic functionality with a simple unit test
	cfg := &config.Config{
		Upstreams: []config.Upstream{
			{Name: "test", Address: "localhost:5672"},
		},
	}
	server := trabbits.NewTestServer(cfg)
	proxy := server.NewProxy(nil)
	proxy.SetUser("testuser")
	proxy.SetVirtualHost("/test")

	// Simulate some method calls
	if proxy.Stats() != nil {
		proxy.Stats().IncrementMethod("Basic.Publish")
		proxy.Stats().IncrementMethod("Basic.Publish")
		proxy.Stats().IncrementMethod("Basic.Consume")
		proxy.Stats().IncrementReceivedFrames()
		proxy.Stats().IncrementReceivedFrames()
		proxy.Stats().IncrementSentFrames()
	}

	// Check if stats are correctly updated
	if proxy.Stats().GetTotalMethods() != 3 {
		t.Errorf("Expected total methods to be 3, got %d", proxy.Stats().GetTotalMethods())
	}
	if proxy.Stats().GetTotalFrames() != 3 {
		t.Errorf("Expected total frames to be 3, got %d", proxy.Stats().GetTotalFrames())
	}
	if proxy.Stats().GetMethodCount("Basic.Publish") != 2 {
		t.Errorf("Expected Basic.Publish count to be 2, got %d", proxy.Stats().GetMethodCount("Basic.Publish"))
	}
}

// TestProxyStatsInClientInfo verifies that stats are included in the clients list
func TestProxyStatsInClientInfo(t *testing.T) {
	cfg := &config.Config{
		Upstreams: []config.Upstream{
			{Name: "test", Address: "localhost:5672"},
		},
	}
	server := trabbits.NewTestServer(cfg)

	// Create a proxy with some activity
	proxy := server.NewProxy(nil)
	proxy.SetUser("testuser")
	proxy.SetVirtualHost("/test")

	// Simulate some method calls
	if proxy.Stats() != nil {
		proxy.Stats().IncrementMethod("Basic.Publish")
		proxy.Stats().IncrementMethod("Queue.Declare")
		for i := 0; i < 6; i++ {
			proxy.Stats().IncrementReceivedFrames()
		}
		for i := 0; i < 4; i++ {
			proxy.Stats().IncrementSentFrames()
		}
	}

	server.RegisterProxy(proxy, func() {})
	defer server.UnregisterProxy(proxy)

	// Get clients info
	clients := server.GetClientsInfo()
	if len(clients) != 1 {
		t.Fatalf("Expected 1 client, got %d", len(clients))
	}

	client := clients[0]
	if client.Stats == nil {
		t.Fatal("Expected stats to be present in ClientInfo")
	}

	if client.Stats.TotalMethods != 2 {
		t.Errorf("Expected total methods to be 2, got %d", client.Stats.TotalMethods)
	}
	if client.Stats.TotalFrames != 10 {
		t.Errorf("Expected total frames to be 10, got %d", client.Stats.TotalFrames)
	}
	if client.Stats.Duration == "" {
		t.Error("Expected duration to be non-empty")
	}
}
