// MIT License
// Copyright (c) 2025 FUJIWARA Shunichiro

package trabbits

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	rabbitmq "github.com/rabbitmq/amqp091-go"
)

type HealthStatus int

const (
	StatusUnknown HealthStatus = iota
	StatusHealthy
	StatusUnhealthy
)

func (s HealthStatus) String() string {
	switch s {
	case StatusHealthy:
		return "healthy"
	case StatusUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// NodeStatus represents the health status of a single cluster node
type NodeStatus struct {
	Address          string
	Status           HealthStatus
	LastCheck        time.Time
	LastSuccess      time.Time
	ConsecutiveFails int
	mu               sync.RWMutex
}

func (n *NodeStatus) GetStatus() HealthStatus {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.Status
}

func (n *NodeStatus) SetHealthy() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Status = StatusHealthy
	n.LastCheck = time.Now()
	n.LastSuccess = time.Now()
	n.ConsecutiveFails = 0
}

func (n *NodeStatus) SetUnhealthy() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Status = StatusUnhealthy
	n.LastCheck = time.Now()
	n.ConsecutiveFails++
}

func (n *NodeStatus) IncrementFails() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.ConsecutiveFails++
	n.LastCheck = time.Now()
}

// InternalHealthCheckConfig uses native time.Duration for internal use
type InternalHealthCheckConfig struct {
	Enabled            bool
	Interval           time.Duration
	Timeout            time.Duration
	UnhealthyThreshold int
	RecoveryInterval   time.Duration
}

// NodeHealthManager manages health checking for cluster nodes
type NodeHealthManager struct {
	name           string
	nodes          map[string]*NodeStatus // address -> status
	mu             sync.RWMutex
	config         *InternalHealthCheckConfig
	upstreamConfig *UpstreamConfig
	logger         *slog.Logger
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

// NewNodeHealthManager creates a new health manager for a cluster upstream
func NewNodeHealthManager(upstream UpstreamConfig) *NodeHealthManager {
	if upstream.Cluster == nil {
		return nil
	}

	mgr := &NodeHealthManager{
		name:           upstream.Name,
		nodes:          make(map[string]*NodeStatus),
		upstreamConfig: &upstream,
		logger:         slog.Default().With("component", "health", "upstream", upstream.Name),
	}

	// Initialize health check config with defaults
	mgr.config = &InternalHealthCheckConfig{
		Enabled:            true,
		Interval:           30 * time.Second,
		Timeout:            5 * time.Second,
		UnhealthyThreshold: 3,
		RecoveryInterval:   60 * time.Second,
	}

	// Override with user config if provided
	if upstream.HealthCheck != nil {
		if upstream.HealthCheck.Interval > 0 {
			mgr.config.Interval = upstream.HealthCheck.Interval.ToDuration()
		}
		if upstream.HealthCheck.Timeout > 0 {
			mgr.config.Timeout = upstream.HealthCheck.Timeout.ToDuration()
		}
		if upstream.HealthCheck.UnhealthyThreshold > 0 {
			mgr.config.UnhealthyThreshold = upstream.HealthCheck.UnhealthyThreshold
		}
		if upstream.HealthCheck.RecoveryInterval > 0 {
			mgr.config.RecoveryInterval = upstream.HealthCheck.RecoveryInterval.ToDuration()
		}
		mgr.config.Enabled = upstream.HealthCheck.Enabled
	}

	// Initialize node status for each cluster node
	for _, node := range upstream.Cluster.Nodes {
		addr := fmt.Sprintf("%s:%d", node.Host, node.Port)
		mgr.nodes[addr] = &NodeStatus{
			Address: addr,
			Status:  StatusUnknown,
		}
	}

	return mgr
}

// StartHealthCheck starts the background health checking goroutine
func (m *NodeHealthManager) StartHealthCheck(ctx context.Context) {
	if m == nil || !m.config.Enabled {
		return
	}

	ctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.logger.Info("Starting health check",
			"interval", m.config.Interval,
			"timeout", m.config.Timeout,
			"threshold", m.config.UnhealthyThreshold)

		// Initial check
		m.checkAllNodes()

		ticker := time.NewTicker(m.config.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				m.checkAllNodes()
			case <-ctx.Done():
				m.logger.Info("Stopping health check")
				return
			}
		}
	}()
}

// Stop stops the health checking goroutine
func (m *NodeHealthManager) Stop() {
	if m == nil || m.cancel == nil {
		return
	}
	m.cancel()
	m.wg.Wait()
}

// GetHealthyNodes returns a list of addresses for healthy nodes
func (m *NodeHealthManager) GetHealthyNodes() []string {
	if m == nil {
		return nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var healthy []string
	for addr, status := range m.nodes {
		s := status.GetStatus()
		if s == StatusHealthy || s == StatusUnknown {
			healthy = append(healthy, addr)
		}
	}

	// If no healthy nodes, return all nodes as fallback
	if len(healthy) == 0 {
		m.logger.Warn("No healthy nodes found, returning all nodes")
		for addr := range m.nodes {
			healthy = append(healthy, addr)
		}
	}

	return healthy
}

// checkAllNodes performs health checks on all nodes
func (m *NodeHealthManager) checkAllNodes() {
	m.mu.RLock()
	nodes := make([]*NodeStatus, 0, len(m.nodes))
	for _, node := range m.nodes {
		nodes = append(nodes, node)
	}
	m.mu.RUnlock()

	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go func(n *NodeStatus) {
			defer wg.Done()
			m.checkNode(n)
		}(node)
	}
	wg.Wait()

	// Log summary
	healthy, unhealthy := m.getHealthSummary()
	m.logger.Debug("Health check completed",
		"healthy", healthy,
		"unhealthy", unhealthy,
		"total", len(nodes))

	// Update metrics
	metrics.UpstreamHealthyNodes.WithLabelValues(m.name).Set(float64(healthy))
	metrics.UpstreamUnhealthyNodes.WithLabelValues(m.name).Set(float64(unhealthy))
}

// checkNode performs a health check on a single node
func (m *NodeHealthManager) checkNode(node *NodeStatus) {
	_, cancel := context.WithTimeout(context.Background(), m.config.Timeout)
	defer cancel()

	// Skip check for recently checked unhealthy nodes (recovery interval)
	if node.GetStatus() == StatusUnhealthy {
		node.mu.RLock()
		timeSinceLastCheck := time.Since(node.LastCheck)
		node.mu.RUnlock()

		if timeSinceLastCheck < m.config.RecoveryInterval {
			return
		}
	}

	// Try to connect to the node
	u := &url.URL{
		Scheme: "amqp",
		User:   url.UserPassword("guest", "guest"), // Use default credentials for health check
		Host:   node.Address,
		Path:   "/",
	}

	cfg := rabbitmq.Config{
		Dial: rabbitmq.DefaultDial(m.config.Timeout),
	}

	conn, err := rabbitmq.DialConfig(u.String(), cfg)
	if err != nil {
		node.IncrementFails()

		node.mu.RLock()
		fails := node.ConsecutiveFails
		node.mu.RUnlock()

		if fails >= m.config.UnhealthyThreshold {
			previousStatus := node.GetStatus()
			node.SetUnhealthy()
			if previousStatus != StatusUnhealthy {
				m.logger.Warn("Node marked as unhealthy",
					"address", node.Address,
					"consecutive_fails", fails,
					"error", err)
			}
		}
		return
	}

	// Connection successful, close it immediately
	conn.Close()

	previousStatus := node.GetStatus()
	node.SetHealthy()

	switch previousStatus {
	case StatusUnhealthy:
		m.logger.Info("Node recovered", "address", node.Address)
	case StatusUnknown:
		m.logger.Info("Node marked as healthy", "address", node.Address)
	}
}

// getHealthSummary returns the count of healthy and unhealthy nodes
func (m *NodeHealthManager) getHealthSummary() (healthy, unhealthy int) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, status := range m.nodes {
		switch status.GetStatus() {
		case StatusHealthy, StatusUnknown:
			healthy++
		case StatusUnhealthy:
			unhealthy++
		}
	}
	return
}
