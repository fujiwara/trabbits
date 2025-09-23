package trabbits_test

import (
	"testing"

	"github.com/fujiwara/trabbits"
	"github.com/fujiwara/trabbits/config"
)

func TestSortNodesByLeastConnections(t *testing.T) {
	// Create a server and proxy instance to test the new method
	cfg := &config.Config{}
	server := trabbits.NewTestServer(cfg)
	proxy := server.NewProxy(nil)

	tests := []struct {
		name             string
		nodes            []string
		connectionCounts map[string]int
		want             []string // First node should have least connections
	}{
		{
			name:  "single node",
			nodes: []string{"node1:5672"},
			connectionCounts: map[string]int{
				"node1:5672": 5,
			},
			want: []string{"node1:5672"},
		},
		{
			name:  "two nodes different counts",
			nodes: []string{"node1:5672", "node2:5672"},
			connectionCounts: map[string]int{
				"node1:5672": 10,
				"node2:5672": 5,
			},
			want: []string{"node2:5672", "node1:5672"},
		},
		{
			name:  "three nodes ascending order",
			nodes: []string{"node1:5672", "node2:5672", "node3:5672"},
			connectionCounts: map[string]int{
				"node1:5672": 1,
				"node2:5672": 5,
				"node3:5672": 10,
			},
			want: []string{"node1:5672", "node2:5672", "node3:5672"},
		},
		{
			name:  "three nodes descending order",
			nodes: []string{"node1:5672", "node2:5672", "node3:5672"},
			connectionCounts: map[string]int{
				"node1:5672": 10,
				"node2:5672": 5,
				"node3:5672": 1,
			},
			want: []string{"node3:5672", "node2:5672", "node1:5672"},
		},
		{
			name:             "empty slice",
			nodes:            []string{},
			connectionCounts: map[string]int{},
			want:             []string{},
		},
		{
			name:  "zero connections",
			nodes: []string{"node1:5672", "node2:5672"},
			connectionCounts: map[string]int{
				"node1:5672": 0,
				"node2:5672": 0,
			},
			// Both nodes have 0 connections, so either order is valid
			// We'll just check that both nodes are present
			want: nil, // Special case - we'll check this separately
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics
			server.Metrics().UpstreamConnections.Reset()

			// Set up connection counts
			for addr, count := range tt.connectionCounts {
				gauge := server.Metrics().UpstreamConnections.WithLabelValues(addr)
				gauge.Set(float64(count))
			}

			result := proxy.SortNodesByLeastConnections(tt.nodes)

			if tt.want == nil && tt.name == "zero connections" {
				// Special case: check that all nodes are present
				if len(result) != len(tt.nodes) {
					t.Errorf("Expected %d nodes, got %d", len(tt.nodes), len(result))
				}
				nodeSet := make(map[string]bool)
				for _, node := range result {
					nodeSet[node] = true
				}
				for _, expectedNode := range tt.nodes {
					if !nodeSet[expectedNode] {
						t.Errorf("Expected node %s not found in result", expectedNode)
					}
				}
				return
			}

			if len(result) != len(tt.want) {
				t.Errorf("Expected %d nodes, got %d", len(tt.want), len(result))
				return
			}

			for i, expectedNode := range tt.want {
				if result[i] != expectedNode {
					t.Errorf("Expected node[%d] = %s, got %s", i, expectedNode, result[i])
				}
			}
		})
	}
}

func TestSortNodesByLeastConnectionsOrder(t *testing.T) {
	// Test that nodes with same connection count can appear in any order
	// but nodes with fewer connections always come first

	// Create a server and proxy instance
	cfg := &config.Config{}
	server := trabbits.NewTestServer(cfg)
	proxy := server.NewProxy(nil)

	nodes := []string{"low1:5672", "low2:5672", "high1:5672", "high2:5672"}
	connectionCounts := map[string]int{
		"low1:5672":  1,
		"low2:5672":  1, // Same as low1
		"high1:5672": 10,
		"high2:5672": 10, // Same as high1
	}

	// Reset metrics and set up connection counts
	server.Metrics().UpstreamConnections.Reset()
	for addr, count := range connectionCounts {
		gauge := server.Metrics().UpstreamConnections.WithLabelValues(addr)
		gauge.Set(float64(count))
	}

	// Run multiple times to check randomization of same-count nodes
	for i := 0; i < 10; i++ {
		result := proxy.SortNodesByLeastConnections(nodes)

		if len(result) != 4 {
			t.Fatalf("Expected 4 nodes, got %d", len(result))
		}

		// First two nodes should be the ones with 1 connection
		firstTwoConnections := make([]int, 2)
		for j := 0; j < 2; j++ {
			firstTwoConnections[j] = connectionCounts[result[j]]
		}
		if firstTwoConnections[0] != 1 || firstTwoConnections[1] != 1 {
			t.Errorf("First two nodes should have 1 connection each, got connections: %v", firstTwoConnections)
		}

		// Last two nodes should be the ones with 10 connections
		lastTwoConnections := make([]int, 2)
		for j := 2; j < 4; j++ {
			lastTwoConnections[j-2] = connectionCounts[result[j]]
		}
		if lastTwoConnections[0] != 10 || lastTwoConnections[1] != 10 {
			t.Errorf("Last two nodes should have 10 connections each, got connections: %v", lastTwoConnections)
		}
	}
}

func TestSortNodesByLeastConnectionsEdgeCases(t *testing.T) {
	// Create a server and proxy instance
	cfg := &config.Config{}
	server := trabbits.NewTestServer(cfg)
	proxy := server.NewProxy(nil)

	t.Run("nil slice", func(t *testing.T) {
		result := proxy.SortNodesByLeastConnections(nil)
		if result != nil {
			t.Errorf("Expected nil, got %v", result)
		}
	})

	t.Run("single node with high connections", func(t *testing.T) {
		server.Metrics().UpstreamConnections.Reset()
		nodes := []string{"busy:5672"}
		gauge := server.Metrics().UpstreamConnections.WithLabelValues("busy:5672")
		gauge.Set(1000)

		result := proxy.SortNodesByLeastConnections(nodes)
		expected := []string{"busy:5672"}

		if len(result) != len(expected) {
			t.Errorf("Expected %v, got %v", expected, result)
		}
		if result[0] != expected[0] {
			t.Errorf("Expected %s, got %s", expected[0], result[0])
		}
	})

	t.Run("all nodes equal connections", func(t *testing.T) {
		server.Metrics().UpstreamConnections.Reset()
		nodes := []string{"node1:5672", "node2:5672", "node3:5672", "node4:5672"}

		// Set all nodes to same connection count
		for _, node := range nodes {
			gauge := server.Metrics().UpstreamConnections.WithLabelValues(node)
			gauge.Set(5)
		}

		result := proxy.SortNodesByLeastConnections(nodes)

		// Should contain all nodes
		if len(result) != len(nodes) {
			t.Errorf("Expected %d nodes, got %d", len(nodes), len(result))
		}

		// Check all original nodes are present
		resultSet := make(map[string]bool)
		for _, node := range result {
			resultSet[node] = true
		}

		for _, originalNode := range nodes {
			if !resultSet[originalNode] {
				t.Errorf("Missing node %s in result", originalNode)
			}
		}
	})
}
