// MIT License
// Copyright (c) 2025 FUJIWARA Shunichiro

package health

import (
	"context"
	"log/slog"
	"net"
	"net/url"
	"runtime/debug"
	"sync"
	"time"

	"github.com/fujiwara/trabbits/config"
	rabbitmq "github.com/rabbitmq/amqp091-go"
)

// dialWithTCPNoDelay creates a dialer function that enables TCP_NODELAY
func dialWithTCPNoDelay(timeout time.Duration) func(network, addr string) (net.Conn, error) {
	return func(network, addr string) (net.Conn, error) {
		conn, err := net.DialTimeout(network, addr, timeout)
		if err != nil {
			return nil, err
		}

		// Enable TCP_NODELAY to disable Nagle's algorithm for lower latency
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			if err := tcpConn.SetNoDelay(true); err != nil {
				slog.Warn("Failed to set TCP_NODELAY on health check connection", "error", err)
			}
		}

		return conn, nil
	}
}

// recoverFromPanic recovers from panic and logs the details
func recoverFromPanic(logger *slog.Logger, functionName string) {
	if r := recover(); r != nil {
		logger.Error("panic recovered",
			"function", functionName,
			"panic", r,
			"stack", string(debug.Stack()))
	}
}

type Status int

const (
	StatusUnknown Status = iota
	StatusHealthy
	StatusUnhealthy
)

func (s Status) String() string {
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
	Status           Status
	LastCheck        time.Time
	LastSuccess      time.Time
	ConsecutiveFails int
	mu               sync.RWMutex
}

func (n *NodeStatus) GetStatus() Status {
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

// Config represents health check configuration
type Config struct {
	Interval           time.Duration
	Timeout            time.Duration
	UnhealthyThreshold int
	RecoveryInterval   time.Duration
}

// MetricsReporter interface for reporting health metrics
type MetricsReporter interface {
	SetHealthyNodes(upstream string, count float64)
	SetUnhealthyNodes(upstream string, count float64)
}

// Manager manages health checking for cluster nodes
type Manager struct {
	name     string
	nodes    map[string]*NodeStatus // address -> status
	mu       sync.RWMutex
	config   *Config
	logger   *slog.Logger
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	username string
	password string
	metrics  MetricsReporter
}

// NewManager creates a new health manager from upstream config
func NewManager(upstream *config.Upstream, metrics MetricsReporter) *Manager {
	if upstream.Cluster == nil || upstream.HealthCheck == nil {
		return nil
	}

	if len(upstream.Cluster.Nodes) == 0 {
		return nil
	}

	// Initialize health check config with defaults
	healthConfig := &Config{
		Interval:           30 * time.Second,
		Timeout:            5 * time.Second,
		UnhealthyThreshold: 3,
		RecoveryInterval:   60 * time.Second,
	}

	// Override with user config if provided
	if upstream.HealthCheck.Interval > 0 {
		healthConfig.Interval = upstream.HealthCheck.Interval.ToDuration()
	}
	if upstream.HealthCheck.Timeout > 0 {
		healthConfig.Timeout = upstream.HealthCheck.Timeout.ToDuration()
	}
	if upstream.HealthCheck.UnhealthyThreshold > 0 {
		healthConfig.UnhealthyThreshold = upstream.HealthCheck.UnhealthyThreshold
	}
	if upstream.HealthCheck.RecoveryInterval > 0 {
		healthConfig.RecoveryInterval = upstream.HealthCheck.RecoveryInterval.ToDuration()
	}

	mgr := &Manager{
		name:     upstream.Name,
		nodes:    make(map[string]*NodeStatus),
		config:   healthConfig,
		logger:   slog.Default().With("component", "health", "upstream", upstream.Name),
		username: upstream.HealthCheck.Username,
		password: upstream.HealthCheck.Password.String(),
		metrics:  metrics,
	}

	// Initialize node status for each cluster node
	for _, addr := range upstream.Cluster.Nodes {
		mgr.nodes[addr] = &NodeStatus{
			Address: addr,
			Status:  StatusUnknown,
		}
	}

	return mgr
}

// StartHealthCheck starts the background health checking goroutine
// It performs an initial health check before starting the background routine
func (m *Manager) StartHealthCheck(ctx context.Context) {
	if m == nil {
		return
	}

	m.logger.Info("Starting health check",
		"interval", m.config.Interval,
		"timeout", m.config.Timeout,
		"threshold", m.config.UnhealthyThreshold)

	// Perform initial health check synchronously (fail fast on startup)
	m.logger.Debug("Performing initial health check")
	m.checkAllNodesInitial()
	m.logger.Debug("Initial health check completed")

	ctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel

	m.wg.Add(1)
	go func() {
		defer recoverFromPanic(m.logger, "health.monitor")
		defer m.wg.Done()

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
func (m *Manager) Stop() {
	if m == nil || m.cancel == nil {
		return
	}
	m.cancel()
	m.wg.Wait()
}

// GetHealthyNodes returns a list of addresses for healthy nodes
func (m *Manager) GetHealthyNodes() []string {
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
func (m *Manager) checkAllNodes() {
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
			m.checkNode(n, false)
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
	if m.metrics != nil {
		m.metrics.SetHealthyNodes(m.name, float64(healthy))
		m.metrics.SetUnhealthyNodes(m.name, float64(unhealthy))
	}
}

// checkAllNodesInitial performs initial health checks on all nodes (fail fast on startup)
func (m *Manager) checkAllNodesInitial() {
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
			m.checkNode(n, true)
		}(node)
	}
	wg.Wait()

	// Log summary
	healthy, unhealthy := m.getHealthSummary()
	m.logger.Debug("Initial health check completed",
		"healthy", healthy,
		"unhealthy", unhealthy,
		"total", len(nodes))

	// Update metrics
	if m.metrics != nil {
		m.metrics.SetHealthyNodes(m.name, float64(healthy))
		m.metrics.SetUnhealthyNodes(m.name, float64(unhealthy))
	}
}

// checkNode performs a health check on a single node
func (m *Manager) checkNode(node *NodeStatus, failFast bool) {
	_, cancel := context.WithTimeout(context.Background(), m.config.Timeout)
	defer cancel()

	// Determine the failure threshold based on mode
	threshold := m.config.UnhealthyThreshold
	if failFast {
		threshold = 1
	}

	// Skip check for recently checked unhealthy nodes (recovery interval)
	// This only applies to regular health checks, not fail-fast checks
	if !failFast && node.GetStatus() == StatusUnhealthy {
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
		User:   url.UserPassword(m.username, m.password),
		Host:   node.Address,
		Path:   "/",
	}

	cfg := rabbitmq.Config{
		Dial: dialWithTCPNoDelay(m.config.Timeout),
	}

	conn, err := rabbitmq.DialConfig(u.String(), cfg)
	if err != nil {
		node.IncrementFails()

		node.mu.RLock()
		fails := node.ConsecutiveFails
		node.mu.RUnlock()

		m.logger.Debug("Health check failed",
			"address", node.Address,
			"consecutive_fails", fails,
			"threshold", threshold,
			"error", err)

		if fails >= threshold {
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

	m.logger.Debug("Health check succeeded", "address", node.Address)

	switch previousStatus {
	case StatusUnhealthy:
		m.logger.Info("Node recovered", "address", node.Address)
	case StatusUnknown:
		m.logger.Info("Node marked as healthy", "address", node.Address)
	}
}

// getHealthSummary returns the count of healthy and unhealthy nodes
func (m *Manager) getHealthSummary() (healthy, unhealthy int) {
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
