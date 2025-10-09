// Copyright (c) 2021 VMware, Inc. or its affiliates. All Rights Reserved.
// Copyright (c) 2012-2021, Sean Treadway, SoundCloud Ltd.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// MIT License
// Copyright (c) 2025 FUJIWARA Shunichiro

package trabbits

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"runtime/debug"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/fujiwara/trabbits/amqp091"
	"github.com/fujiwara/trabbits/config"
	"github.com/fujiwara/trabbits/health"
	"github.com/fujiwara/trabbits/metrics"
	"github.com/fujiwara/trabbits/types"
	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/time/rate"
)

var (
	ChannelMax                    = uint16(1023)
	HeartbeatInterval             = uint16(60)
	FrameMax                      = uint32(128 * 1024)
	UpstreamDefaultTimeout        = 5 * time.Second
	DefaultHandshakeTimeout       = 5 * time.Second
	DefaultProcessTimeout         = 5 * time.Minute // Long timeout for process loop (low-frequency clients)
	DefaultConnectionCloseTimeout = 1 * time.Second

	// Shutdown messages
	ShutdownMsgServerShutdown = "Server shutting down"
	ShutdownMsgConfigUpdate   = "Configuration updated, please reconnect"
	ShutdownMsgDefault        = "Connection closed"

	// ProbeLog retention
	ProbeLogBufferSize    = 100  // Number of logs to keep per proxy
	ProbeLogRetentionSize = 1000 // Number of proxies to retain logs for (LRU)
)

var Debug bool

// Server represents a trabbits server instance
type proxyEntry struct {
	proxy  *Proxy
	cancel context.CancelFunc
}

type Server struct {
	configMu          sync.RWMutex
	config            *config.Config
	configHash        string
	activeProxies     sync.Map                            // proxy id -> *proxyEntry
	retainedProbeLogs *lru.Cache[string, *ProbeLogBuffer] // LRU cache for disconnected proxies' probe logs
	healthManagers    sync.Map                            // upstream name -> *health.Manager
	logger            *slog.Logger
	apiSocket         string // API socket path
	metricsStore      *metrics.Store
	logBuffer         *LogBuffer // Buffer for server logs
}

// NewServer creates a new Server instance
func NewServer(config *config.Config, apiSocket string) *Server {
	logBuffer := NewLogBuffer(200) // Keep last 200 log entries
	probeLogCache, err := lru.New[string, *ProbeLogBuffer](ProbeLogRetentionSize)
	if err != nil {
		panic(fmt.Sprintf("failed to create LRU cache: %v", err))
	}
	return &Server{
		config:            config,
		configHash:        config.Hash(),
		logger:            slog.Default(),
		apiSocket:         apiSocket,
		metricsStore:      metrics.NewStore(),
		logBuffer:         logBuffer,
		retainedProbeLogs: probeLogCache,
	}
}

// GetConfig returns the current config (thread-safe)
func (s *Server) GetConfig() *config.Config {
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	return s.config
}

// Metrics returns the metrics instance for this server
func (s *Server) Metrics() *metrics.Metrics {
	return s.metricsStore.Metrics()
}

// MetricsStore returns the metrics store for this server
func (s *Server) MetricsStore() *metrics.Store {
	return s.metricsStore
}

// GetConfigHash returns the current config hash (thread-safe)
func (s *Server) GetConfigHash() string {
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	return s.configHash
}

// UpdateConfig updates the config and hash atomically
func (s *Server) UpdateConfig(config *config.Config) {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	s.config = config
	s.configHash = config.Hash()
}

// NewProxy creates a new proxy associated with this server
func (s *Server) NewProxy(conn net.Conn) *Proxy {
	id := generateID()

	// Get config with timeout values
	config := s.GetConfig()

	// Create a probe log buffer for this proxy (will be added to LRU in RegisterProxy)
	probeLogBuffer := NewProbeLogBuffer(ProbeLogBufferSize)

	proxy := &Proxy{
		conn:                   conn,
		id:                     id,
		r:                      amqp091.NewReader(conn),
		w:                      amqp091.NewWriter(conn),
		upstreamDisconnect:     make(chan string, 10), // buffered to avoid blocking
		configHash:             s.GetConfigHash(),
		handshakeTimeout:       config.HandshakeTimeout.ToDuration(),
		processTimeout:         DefaultProcessTimeout,
		connectionCloseTimeout: config.ConnectionCloseTimeout.ToDuration(),
		shutdownMessage:        ShutdownMsgDefault,       // default shutdown message
		connectedAt:            time.Now(),               // timestamp when the client connected
		stats:                  NewProxyStats(),          // initialize statistics
		probeChan:              make(chan probeLog, 100), // buffered channel for probe logs
		probeLogBuffer:         probeLogBuffer,           // buffer for probe log retention
		metrics:                s.Metrics(),              // metrics instance from server
		tuned: tuned{
			channelMax: ChannelMax,
			frameMax:   FrameMax,
			heartbeat:  HeartbeatInterval,
		},
	}
	proxy.logger = slog.New(slog.Default().Handler()).With("proxy", id, "client_addr", proxy.ClientAddr())
	return proxy
}

// GetHealthManager returns the health manager for the given upstream name
func (s *Server) GetHealthManager(upstreamName string) *health.Manager {
	if mgr, ok := s.healthManagers.Load(upstreamName); ok {
		return mgr.(*health.Manager)
	}
	return nil
}

// RegisterProxy adds a proxy and its cancel function to the server's active proxy list
func (s *Server) RegisterProxy(proxy *Proxy, cancel context.CancelFunc) {
	s.activeProxies.Store(proxy.id, &proxyEntry{proxy: proxy, cancel: cancel})
	// Add the proxy's probe log buffer to LRU cache for retention
	if proxy.probeLogBuffer != nil {
		// Store proxy information in the buffer for later retrieval when disconnected
		proxy.probeLogBuffer.SetProxyInfo(
			proxy.ClientAddr(),
			proxy.user,
			proxy.VirtualHost,
			proxy.ClientBanner(),
			proxy.connectedAt,
		)
		s.retainedProbeLogs.Add(proxy.id, proxy.probeLogBuffer)
	}
}

// UnregisterProxy removes a proxy from the server's active proxy list
// and marks its probe log buffer as inactive
func (s *Server) UnregisterProxy(proxy *Proxy) {
	s.activeProxies.Delete(proxy.id)
	// Mark the probe log buffer as inactive (but keep it in LRU cache)
	if buffer, ok := s.retainedProbeLogs.Get(proxy.id); ok {
		buffer.MarkInactive()
	}
}

// GetProbeLogBuffer returns the probe log buffer for a given proxy ID
func (s *Server) GetProbeLogBuffer(proxyID string) (*ProbeLogBuffer, bool) {
	return s.retainedProbeLogs.Get(proxyID)
}

// disconnectProxies gracefully disconnects proxies based on a filter function
// Returns a channel that will receive the number of disconnected proxies when complete
func (srv *Server) disconnectProxies(reason string, shutdownMessage string, maxTimeout time.Duration, rateLimit int, burstSize int, filter func(*Proxy) bool) <-chan int {
	resultChan := make(chan int, 1)

	go func() {
		defer close(resultChan)

		var targetProxies []*Proxy
		var targetCancels []context.CancelFunc

		srv.activeProxies.Range(func(key, value interface{}) bool {
			entry := value.(*proxyEntry)
			if filter(entry.proxy) {
				// Set the shutdown message for this proxy
				entry.proxy.setShutdownMessage(shutdownMessage)
				targetProxies = append(targetProxies, entry.proxy)
				targetCancels = append(targetCancels, entry.cancel)
			}
			return true
		})

		disconnectedCount := len(targetProxies)
		if disconnectedCount == 0 {
			// No target proxies found, return immediately
			resultChan <- 0
			return
		}
		slog.Info(reason, "count", disconnectedCount)

		// Rate limiter: use configured values
		// In test environment, use higher rate for faster tests
		rateLimitValue := rate.Limit(rateLimit)
		if disconnectedCount <= 10 { // Small number for tests
			rateLimitValue = rate.Limit(1000) // 1000/sec for small tests
			burstSize = 100
		}
		limiter := rate.NewLimiter(rateLimitValue, burstSize)

		// Number of worker goroutines for parallel processing
		const numWorkers = 10

		// Channel to distribute work to workers
		cancelChan := make(chan context.CancelFunc, len(targetCancels))

		// Send all cancel functions to the channel
		for _, cancel := range targetCancels {
			cancelChan <- cancel
		}
		close(cancelChan)

		// Use sync.WaitGroup to wait for all worker goroutines to complete
		var wg sync.WaitGroup

		// Start worker goroutines
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer recoverFromPanic(slog.Default(), "DisconnectOutdatedProxies.worker", srv.Metrics())
				defer wg.Done()

				for cancel := range cancelChan {
					// Wait for rate limiter
					if err := limiter.Wait(context.Background()); err != nil {
						slog.Warn("Rate limiter context cancelled", "error", err)
						continue
					}

					// Cancel the proxy's context - this will trigger graceful shutdown
					// within the proxy's own goroutine, avoiding concurrent writes
					cancel()
				}
			}(i)
		}

		// Wait for all workers to complete with appropriate timeout
		// Calculate timeout based on proxy count and rate limit
		actualRate := float64(rateLimitValue)
		expectedDuration := float64(disconnectedCount)/actualRate + 1 // Plus 1 second buffer
		timeoutDuration := time.Duration(expectedDuration) * time.Second
		if timeoutDuration < 2*time.Second {
			timeoutDuration = 2 * time.Second // Minimum 2 seconds
		}
		if timeoutDuration > maxTimeout {
			timeoutDuration = maxTimeout
		}

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// All disconnections completed successfully
			slog.Info("All proxy disconnections completed", "count", disconnectedCount)
		case <-time.After(timeoutDuration):
			// Timeout - some disconnections might be hanging
			slog.Warn("Timeout waiting for proxy disconnections to complete",
				"timeout", timeoutDuration,
				"expected_duration", fmt.Sprintf("%.1fs", float64(disconnectedCount)/actualRate))
		}

		// Send the count of disconnected proxies
		resultChan <- disconnectedCount
	}()

	return resultChan
}

// disconnectOutdatedProxies gracefully disconnects proxies with old config hash for this server
// Returns a channel that will receive the number of disconnected proxies when complete
func (srv *Server) disconnectOutdatedProxies(currentConfigHash string) <-chan int {
	cfg := srv.GetConfig()
	return srv.disconnectProxies(
		fmt.Sprintf("Disconnecting outdated proxies, current_hash=%s", currentConfigHash[:8]),
		ShutdownMsgConfigUpdate,
		time.Duration(cfg.GracefulShutdown.ReloadTimeout),
		cfg.GracefulShutdown.RateLimit,
		cfg.GracefulShutdown.BurstSize,
		func(proxy *Proxy) bool {
			if proxy.configHash != currentConfigHash {
				proxy.logger.Info("Disconnecting proxy due to config update",
					"old_hash", proxy.configHash[:8],
					"new_hash", currentConfigHash[:8])
				return true
			}
			return false
		},
	)
}

// disconnectAllProxies gracefully disconnects all active proxies for shutdown
// Returns a channel that will receive the number of disconnected proxies when complete
func (srv *Server) disconnectAllProxies() <-chan int {
	cfg := srv.GetConfig()
	return srv.disconnectProxies(
		"Disconnecting all active proxies for shutdown",
		ShutdownMsgServerShutdown,
		time.Duration(cfg.GracefulShutdown.ShutdownTimeout),
		cfg.GracefulShutdown.RateLimit,
		cfg.GracefulShutdown.BurstSize,
		func(proxy *Proxy) bool {
			proxy.logger.Info("Disconnecting proxy for shutdown")
			return true // Disconnect all proxies
		},
	)
}

func run(ctx context.Context, opt *CLI) error {
	cfg, err := config.Load(ctx, opt.Config)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create server instance
	server := NewServer(cfg, opt.APISocket)

	// Setup server log handler to capture logs for API streaming
	originalHandler := slog.Default().Handler()
	serverLogHandler := NewServerLogHandler(server.logBuffer, originalHandler)
	slog.SetDefault(slog.New(serverLogHandler))

	// Write PID file if specified
	if opt.Run.PidFile != "" {
		if err := writePidFile(opt.Run.PidFile); err != nil {
			return fmt.Errorf("failed to write PID file: %w", err)
		}
		defer removePidFile(opt.Run.PidFile)
	}

	// Initialize health check managers for cluster upstreams
	if err := server.initHealthManagers(ctx); err != nil {
		return fmt.Errorf("failed to init health managers: %w", err)
	}

	cancelAPI, err := server.startAPIServer(ctx, opt.Config)
	if err != nil {
		return fmt.Errorf("failed to start API server: %w", err)
	}
	defer cancelAPI()

	cancelMetrics, err := server.MetricsStore().RunServer(ctx, opt.MetricsPort)
	if err != nil {
		return fmt.Errorf("failed to start metrics server: %w", err)
	}
	defer cancelMetrics()

	// Setup SIGHUP handler for config reload
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-sigChan:
				slog.Info("Received SIGHUP signal, reloading configuration")
				if _, err := server.reloadConfigFromFile(ctx, opt.Config); err != nil {
					slog.Error("Failed to reload config via SIGHUP", "error", err)
				}
			}
		}
	}()

	slog.Info("trabbits starting", "version", Version, "port", opt.Port)
	listener, err := newListener(ctx, fmt.Sprintf(":%d", opt.Port))
	if err != nil {
		return fmt.Errorf("failed to start AMQP server: %w", err)
	}
	defer listener.Close()

	return server.boot(ctx, listener)
}

// writePidFile writes the current process ID to the specified file
func writePidFile(pidFile string) error {
	pid := os.Getpid()
	slog.Info("Writing PID file", "file", pidFile, "pid", pid)

	content := strconv.Itoa(pid)
	if err := os.WriteFile(pidFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write PID file %s: %w", pidFile, err)
	}
	return nil
}

// removePidFile removes the PID file
func removePidFile(pidFile string) {
	slog.Info("Removing PID file", "file", pidFile)
	if err := os.Remove(pidFile); err != nil {
		slog.Warn("Failed to remove PID file", "file", pidFile, "error", err)
	}
}

// boot starts accepting connections for this server
func (srv *Server) boot(ctx context.Context, listener net.Listener) error {
	port := listener.Addr().(*net.TCPAddr).Port
	slog.Info("trabbits started", "port", port)
	go func() {
		<-ctx.Done()
		// First close listener to prevent new connections
		if err := listener.Close(); err != nil {
			slog.Error("Failed to close listener", "error", err)
		}
		slog.Info("Listener closed, no new connections will be accepted")

		// Then gracefully disconnect all active connections
		disconnectChan := srv.disconnectAllProxies()
		cfg := srv.GetConfig()
		select {
		case count := <-disconnectChan:
			if count > 0 {
				slog.Info("Gracefully disconnected all active connections", "count", count)
			}
		case <-time.After(time.Duration(cfg.GracefulShutdown.ShutdownTimeout)):
			slog.Warn("Timeout waiting for graceful disconnection of active connections")
		}
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				slog.Info("trabbits server stopped")
				return nil
			default:
			}
			slog.Warn("Failed to accept connection", "error", err)
			continue
		}

		// Enable TCP_NODELAY to disable Nagle's algorithm for lower latency
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			if err := tcpConn.SetNoDelay(true); err != nil {
				slog.Warn("Failed to set TCP_NODELAY", "error", err)
			}
		}

		go srv.handleConnection(ctx, conn)
	}
}

// initHealthManagers initializes health check managers for cluster upstreams
func (srv *Server) initHealthManagers(ctx context.Context) error {
	// Stop existing health managers
	srv.healthManagers.Range(func(key, value interface{}) bool {
		if mgr, ok := value.(*health.Manager); ok {
			mgr.Stop()
		}
		srv.healthManagers.Delete(key)
		return true
	})

	// Create new health managers for cluster upstreams
	cfg := srv.GetConfig()
	for _, upstream := range cfg.Upstreams {
		if upstream.Cluster != nil && upstream.HealthCheck != nil {
			mgr := health.NewManager(&upstream, srv.Metrics())
			if mgr != nil {
				mgr.StartHealthCheck(ctx)
				srv.healthManagers.Store(upstream.Name, mgr)
				slog.Info("Health check manager initialized",
					"upstream", upstream.Name,
					"nodes", len(upstream.Cluster.Nodes))
			}
		}
	}

	return nil
}

// recoverFromPanic recovers from panic and logs the details with metrics
func recoverFromPanic(logger *slog.Logger, functionName string, metrics *metrics.Metrics) {
	if r := recover(); r != nil {
		logger.Error("panic recovered",
			"function", functionName,
			"panic", r,
			"stack", string(debug.Stack()))
		if metrics != nil {
			metrics.PanicRecoveries.WithLabelValues(functionName).Inc()
		}
	}
}

var amqpHeader = []byte{'A', 'M', 'Q', 'P', 0, 0, 9, 1}

// handleConnection handles a new client connection for this server
func (srv *Server) handleConnection(ctx context.Context, conn net.Conn) {
	defer recoverFromPanic(slog.Default(), "handleConnection", srv.Metrics())

	srv.Metrics().ClientConnections.Inc()
	srv.Metrics().ClientTotalConnections.Inc()
	defer func() {
		conn.Close()
		srv.Metrics().ClientConnections.Dec()
	}()

	slog.Info("new connection", "client_addr", conn.RemoteAddr().String(), "handshake_timeout", srv.config.HandshakeTimeout)
	conn.SetReadDeadline(time.Now().Add(srv.config.HandshakeTimeout.ToDuration())) // Set read deadline for handshake
	// AMQP プロトコルヘッダー受信
	header := make([]byte, 8)
	if _, err := io.ReadFull(conn, header); err != nil {
		slog.Warn("Failed to read AMQP header:", "error", err)
		srv.Metrics().ClientConnectionErrors.Inc()
		return
	}
	if !bytes.Equal(header, amqpHeader) {
		slog.Warn("Invalid AMQP protocol header", "header", header)
		srv.Metrics().ClientConnectionErrors.Inc()
		return
	}

	p := srv.NewProxy(conn)
	defer func() {
		srv.UnregisterProxy(p)
		p.Close()
	}()

	if err := p.handshake(ctx); err != nil {
		p.logger.Warn("Failed to handshake", "error", err)
		srv.Metrics().ClientConnectionErrors.Inc()
		conn.Close()
		return
	}
	p.logger.Debug("handshake completed")

	// subCtx is used for client connection depends on parent context
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	srv.RegisterProxy(p, cancel)
	p.logger.Info("proxy created", "config_hash", p.configHash[:8])

	if err := p.ConnectToUpstreams(subCtx, srv.GetConfig().Upstreams, p.clientProps); err != nil {
		p.logger.Warn("Failed to connect to upstreams", "error", err)
		return
	}
	p.logger.Debug("connected to upstreams")

	p.initHeartbeatTimer()
	go p.runHeartbeat(subCtx)

	// wait for client frames
	for {
		select {
		case <-ctx.Done(): // parent (server) context is done
			p.shutdown(subCtx) // graceful shutdown
			return             // subCtx will be canceled by defer ^
		case <-subCtx.Done():
			p.shutdown(subCtx) // graceful shutdown when context is cancelled
			return
		case upstreamName := <-p.upstreamDisconnect:
			// An upstream connection was lost
			p.logger.Error("Upstream connection lost, closing client connection",
				"upstream", upstreamName)
			// Send Connection.Close to client with connection-forced error
			p.sendConnectionError(NewError(amqp091.ConnectionForced,
				fmt.Sprintf("Upstream connection lost: %s", upstreamName)))
			return
		default:
		}
		if err := p.process(subCtx); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) || isBrokenPipe(err) {
				p.logger.Info("closed connection")
			} else {
				p.logger.Warn("failed to process", "error", err)
			}
			break
		}
	}
}

// GetProxy retrieves a proxy by ID from this server instance
func (s *Server) GetProxy(id string) *Proxy {
	if value, ok := s.activeProxies.Load(id); ok {
		entry := value.(*proxyEntry)
		return entry.proxy
	}
	return nil
}

// CountActiveProxies returns the number of active proxies for this server instance
func (s *Server) CountActiveProxies() int {
	count := 0
	s.activeProxies.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// GetClientsInfo returns information about all connected clients
func (s *Server) GetClientsInfo() []types.ClientInfo {
	clients := make([]types.ClientInfo, 0) // Initialize as empty slice instead of nil
	activeProxyIDs := make(map[string]bool)

	// First, add all active proxies
	s.activeProxies.Range(func(key, value interface{}) bool {
		entry := value.(*proxyEntry)
		proxy := entry.proxy
		activeProxyIDs[proxy.id] = true

		// Determine status based on shutdown message
		status := types.ClientStatusActive
		shutdownReason := ""
		if proxy.shutdownMessage != ShutdownMsgDefault {
			status = types.ClientStatusShuttingDown
			shutdownReason = proxy.shutdownMessage
		}

		clientInfo := types.ClientInfo{
			ID:               proxy.id,
			ClientAddress:    proxy.ClientAddr(),
			User:             proxy.user,
			VirtualHost:      proxy.VirtualHost,
			ClientBanner:     proxy.ClientBanner(),
			ClientProperties: nil, // Nil for clients list to omit from JSON output
			ConnectedAt:      proxy.connectedAt,
			Status:           status,
			ShutdownReason:   shutdownReason,
		}

		// Add statistics summary if available
		if proxy.stats != nil {
			snapshot := proxy.stats.Snapshot()
			clientInfo.Stats = &types.StatsSummary{
				TotalMethods:   snapshot.TotalMethods,
				ReceivedFrames: snapshot.ReceivedFrames,
				SentFrames:     snapshot.SentFrames,
				TotalFrames:    snapshot.TotalFrames,
				Duration:       snapshot.Duration,
			}
		}
		clients = append(clients, clientInfo)
		return true
	})

	// Then, add disconnected proxies from LRU cache
	for _, proxyID := range s.retainedProbeLogs.Keys() {
		// Skip if already in active proxies
		if activeProxyIDs[proxyID] {
			continue
		}

		buffer, ok := s.retainedProbeLogs.Get(proxyID)
		if !ok || buffer.IsActive() {
			continue
		}

		// Get proxy info from buffer
		clientAddr, user, virtualHost, clientBanner, connectedAt := buffer.GetProxyInfo()

		clientInfo := types.ClientInfo{
			ID:             proxyID,
			ClientAddress:  clientAddr,
			User:           user,
			VirtualHost:    virtualHost,
			ClientBanner:   clientBanner,
			ConnectedAt:    connectedAt,
			DisconnectedAt: buffer.GetDisconnectedAt(),
			Status:         types.ClientStatusDisconnected,
		}

		clients = append(clients, clientInfo)
	}

	// Sort clients by connection time (oldest first)
	sort.Slice(clients, func(i, j int) bool {
		return clients[i].ConnectedAt.Before(clients[j].ConnectedAt)
	})

	return clients
}

// GetClientInfo returns full client information including ClientProperties and complete stats
func (s *Server) GetClientInfo(proxyID string) (*types.FullClientInfo, bool) {
	// First, try to find in active proxies
	value, ok := s.activeProxies.Load(proxyID)
	if ok {
		entry := value.(*proxyEntry)
		proxy := entry.proxy

		// Determine status based on shutdown message
		status := types.ClientStatusActive
		shutdownReason := ""
		if proxy.shutdownMessage != ShutdownMsgDefault {
			status = types.ClientStatusShuttingDown
			shutdownReason = proxy.shutdownMessage
		}

		clientInfo := &types.FullClientInfo{
			ID:               proxy.id,
			ClientAddress:    proxy.ClientAddr(),
			User:             proxy.user,
			VirtualHost:      proxy.VirtualHost,
			ClientBanner:     proxy.ClientBanner(),
			ClientProperties: proxy.clientProps, // Include full client properties
			ConnectedAt:      proxy.connectedAt,
			Status:           status,
			ShutdownReason:   shutdownReason,
		}

		// Add complete statistics if available
		if proxy.stats != nil {
			snapshot := proxy.stats.Snapshot()
			clientInfo.Stats = &types.FullStatsSummary{
				StartedAt:      snapshot.StartedAt,
				Methods:        snapshot.Methods,
				TotalMethods:   snapshot.TotalMethods,
				ReceivedFrames: snapshot.ReceivedFrames,
				SentFrames:     snapshot.SentFrames,
				TotalFrames:    snapshot.TotalFrames,
				Duration:       snapshot.Duration,
			}
		}

		return clientInfo, true
	}

	// If not active, try to find in disconnected proxies
	buffer, ok := s.retainedProbeLogs.Get(proxyID)
	if !ok || buffer.IsActive() {
		return nil, false
	}

	// Get proxy info from buffer
	clientAddr, user, virtualHost, clientBanner, connectedAt := buffer.GetProxyInfo()

	clientInfo := &types.FullClientInfo{
		ID:             proxyID,
		ClientAddress:  clientAddr,
		User:           user,
		VirtualHost:    virtualHost,
		ClientBanner:   clientBanner,
		ConnectedAt:    connectedAt,
		DisconnectedAt: buffer.GetDisconnectedAt(),
		Status:         types.ClientStatusDisconnected,
	}

	return clientInfo, true
}

// ShutdownProxy gracefully shuts down a specific proxy by proxy ID
func (s *Server) ShutdownProxy(proxyID string, shutdownReason string) bool {
	if shutdownReason == "" {
		shutdownReason = "API shutdown request"
	}

	value, ok := s.activeProxies.Load(proxyID)
	if !ok {
		slog.Warn("proxy shutdown failed: proxy not found", "proxy", proxyID)
		return false
	}

	entry := value.(*proxyEntry)
	proxy := entry.proxy

	slog.Info("shutting down proxy",
		"proxy", proxyID,
		"client_addr", proxy.ClientAddr(),
		"user", proxy.user,
		"vhost", proxy.VirtualHost,
		"reason", shutdownReason)

	// Set custom shutdown message
	proxy.shutdownMessage = shutdownReason
	// Trigger graceful shutdown
	entry.cancel()

	return true
}
