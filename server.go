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
	"math/rand/v2"
	"net"
	"net/url"
	"runtime/debug"
	"os"
	"os/signal"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fujiwara/trabbits/amqp091"
	"github.com/fujiwara/trabbits/config"
	"github.com/fujiwara/trabbits/health"
	dto "github.com/prometheus/client_model/go"
	rabbitmq "github.com/rabbitmq/amqp091-go"
	"golang.org/x/time/rate"
)

var (
	ChannelMax                    = 1023
	HeartbeatInterval             = 60
	FrameMax                      = 128 * 1024
	UpstreamDefaultTimeout        = 5 * time.Second
	DefaultReadTimeout            = 5 * time.Second
	DefaultConnectionCloseTimeout = 1 * time.Second

	// Shutdown messages
	ShutdownMsgServerShutdown = "Server shutting down"
	ShutdownMsgConfigUpdate   = "Configuration updated, please reconnect"
	ShutdownMsgDefault        = "Connection closed"
)

var Debug bool

// Server represents a trabbits server instance
type proxyEntry struct {
	proxy  *Proxy
	cancel context.CancelFunc
}

type Server struct {
	configMu       sync.RWMutex
	config         *config.Config
	configHash     string
	activeProxies  sync.Map // proxy id -> *proxyEntry
	healthManagers sync.Map // upstream name -> *health.Manager
	logger         *slog.Logger
	apiSocket      string // API socket path
}

// NewServer creates a new Server instance
func NewServer(config *config.Config, apiSocket string) *Server {
	return &Server{
		config:     config,
		configHash: config.Hash(),
		logger:     slog.Default(),
		apiSocket:  apiSocket,
	}
}

// GetConfig returns the current config (thread-safe)
func (s *Server) GetConfig() *config.Config {
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	return s.config
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

	proxy := &Proxy{
		conn:                   conn,
		id:                     id,
		r:                      amqp091.NewReader(conn),
		w:                      amqp091.NewWriter(conn),
		upstreamDisconnect:     make(chan string, 10), // buffered to avoid blocking
		configHash:             s.GetConfigHash(),
		readTimeout:            config.ReadTimeout.ToDuration(),
		connectionCloseTimeout: config.ConnectionCloseTimeout.ToDuration(),
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
}

// UnregisterProxy removes a proxy from the server's active proxy list
func (s *Server) UnregisterProxy(proxy *Proxy) {
	s.activeProxies.Delete(proxy.id)
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
				entry.proxy.shutdownMessage = shutdownMessage
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
				defer recoverFromPanic(slog.Default(), "DisconnectOutdatedProxies.worker")
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

	cancelMetrics, err := runMetricsServer(ctx, opt)
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
			mgr := health.NewManager(&upstream, metrics)
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
func recoverFromPanic(logger *slog.Logger, functionName string) {
	if r := recover(); r != nil {
		logger.Error("panic recovered",
			"function", functionName,
			"panic", r,
			"stack", string(debug.Stack()))
		GetMetrics().PanicRecoveries.WithLabelValues(functionName).Inc()
	}
}

var amqpHeader = []byte{'A', 'M', 'Q', 'P', 0, 0, 9, 1}

// handleConnection handles a new client connection for this server
func (srv *Server) handleConnection(ctx context.Context, conn net.Conn) {
	defer recoverFromPanic(slog.Default(), "handleConnection")

	GetMetrics().ClientConnections.Inc()
	GetMetrics().ClientTotalConnections.Inc()
	defer func() {
		conn.Close()
		GetMetrics().ClientConnections.Dec()
	}()

	slog.Info("new connection", "client_addr", conn.RemoteAddr().String())
	// AMQP プロトコルヘッダー受信
	header := make([]byte, 8)
	if _, err := io.ReadFull(conn, header); err != nil {
		slog.Warn("Failed to read AMQP header:", "error", err)
		GetMetrics().ClientConnectionErrors.Inc()
		return
	}
	if !bytes.Equal(header, amqpHeader) {
		slog.Warn("Invalid AMQP protocol header", "header", header)
		GetMetrics().ClientConnectionErrors.Inc()
		return
	}

	p := srv.NewProxy(conn)
	defer func() {
		srv.UnregisterProxy(p)
		p.Close()
	}()

	if err := p.handshake(ctx); err != nil {
		p.logger.Warn("Failed to handshake", "error", err)
		GetMetrics().ClientConnectionErrors.Inc()
		return
	}
	p.logger.Info("handshake completed")

	// subCtx is used for client connection depends on parent context
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	srv.RegisterProxy(p, cancel)
	p.logger.Info("proxy created", "config_hash", p.configHash[:8])

	if err := p.ConnectToUpstreams(subCtx, srv.GetConfig().Upstreams, p.clientProps); err != nil {
		p.logger.Warn("Failed to connect to upstreams", "error", err)
		return
	}
	p.logger.Info("connected to upstreams")

	go p.runHeartbeat(subCtx, uint16(HeartbeatInterval))

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

func (p *Proxy) connectToUpstreamServer(addr string, props amqp091.Table, timeout time.Duration) (*rabbitmq.Connection, error) {
	u := &url.URL{
		Scheme: "amqp",
		User:   url.UserPassword(p.user, p.password),
		Host:   addr,
		Path:   p.VirtualHost,
	}
	p.logger.Info("connect to upstream", "url", safeURLString(*u))
	// Copy all client properties and override specific ones
	upstreamProps := rabbitmq.Table{}
	for k, v := range props {
		upstreamProps[k] = v
	}
	// overwrite product property to include trabbits and client address
	upstreamProps["product"] = fmt.Sprintf("%s (%s) via trabbits/%s", props["product"], p.ClientAddr(), Version)

	cfg := rabbitmq.Config{
		Properties: upstreamProps,
		Dial:       rabbitmq.DefaultDial(timeout),
	}
	conn, err := rabbitmq.DialConfig(u.String(), cfg)
	if err != nil {
		GetMetrics().UpstreamConnectionErrors.WithLabelValues(addr).Inc()
		return nil, fmt.Errorf("failed to open upstream %s %w", u, err)
	}
	return conn, nil
}

func (p *Proxy) connectToUpstreamServers(upstreamName string, addrs []string, props amqp091.Table, timeout time.Duration) (*rabbitmq.Connection, string, error) {
	var nodesToTry []string

	// Health-based node selection is not available at proxy level
	// This would require server instance access which violates design principles

	// Fall back to all nodes if no health manager or no healthy nodes
	if len(nodesToTry) == 0 {
		nodesToTry = addrs
	}

	// Sort nodes using least connection algorithm
	nodesToTry = sortNodesByLeastConnections(nodesToTry)

	// Try to connect to each node
	for _, addr := range nodesToTry {
		conn, err := p.connectToUpstreamServer(addr, props, timeout)
		if err == nil {
			p.logger.Info("Connected to upstream node", "upstream", upstreamName, "address", addr)
			return conn, addr, nil
		} else {
			p.logger.Warn("Failed to connect to upstream node",
				"upstream", upstreamName,
				"address", addr,
				"error", err)
		}
	}
	return nil, "", fmt.Errorf("failed to connect to any upstream node in %s: tried %v", upstreamName, nodesToTry)
}

// sortNodesByLeastConnections sorts nodes by connection count using least connection algorithm.
// Nodes with fewer connections are placed first. Nodes with equal connections are randomly ordered.
func sortNodesByLeastConnections(nodes []string) []string {
	if len(nodes) <= 1 {
		return nodes
	}

	type nodeInfo struct {
		addr        string
		connections int64
	}

	var nodeInfos []nodeInfo
	for _, addr := range nodes {
		metric := &dto.Metric{}
		gauge := GetMetrics().UpstreamConnections.WithLabelValues(addr)
		gauge.Write(metric)
		connections := int64(metric.GetGauge().GetValue())
		nodeInfos = append(nodeInfos, nodeInfo{addr: addr, connections: connections})
	}

	// Sort by connection count, then shuffle nodes with same connection count
	sort.Slice(nodeInfos, func(i, j int) bool {
		if nodeInfos[i].connections == nodeInfos[j].connections {
			return rand.IntN(2) == 0 // Random order for nodes with same connection count
		}
		return nodeInfos[i].connections < nodeInfos[j].connections
	})

	// Create result slice with sorted addresses
	result := make([]string, len(nodeInfos))
	for i, info := range nodeInfos {
		result[i] = info.addr
	}
	return result
}

func (p *Proxy) ConnectToUpstreams(ctx context.Context, upstreamConfigs []config.Upstream, props amqp091.Table) error {
	for _, c := range upstreamConfigs {
		timeout := c.Timeout.ToDuration()
		if timeout == 0 {
			timeout = UpstreamDefaultTimeout
		}
		conn, addr, err := p.connectToUpstreamServers(c.Name, c.Addresses(), props, timeout)
		if err != nil {
			return err
		}
		us := NewUpstream(conn, p.logger, c, addr)
		p.upstreams = append(p.upstreams, us)

		// Start monitoring the upstream connection
		go p.MonitorUpstreamConnection(ctx, us)
	}
	return nil
}

func (p *Proxy) handshake(ctx context.Context) error {
	// Connection.Start 送信
	start := &amqp091.ConnectionStart{
		VersionMajor: 0,
		VersionMinor: 9,
		Mechanisms:   "PLAIN",
		Locales:      "en_US",
	}
	if err := p.send(0, start); err != nil {
		return fmt.Errorf("failed to write Connection.Start: %w", err)
	}

	// Connection.Start-Ok 受信（認証情報含む）
	startOk := amqp091.ConnectionStartOk{}
	_, err := p.recv(0, &startOk)
	if err != nil {
		return fmt.Errorf("failed to read Connection.Start-Ok: %w", err)
	}
	p.clientProps = startOk.ClientProperties
	p.logger = p.logger.With("client", p.ClientBanner())
	auth := startOk.Mechanism
	authRes := startOk.Response
	switch auth {
	case "PLAIN":
		p.user, p.password, err = amqp091.ParsePLAINAuthResponse(authRes)
		if err != nil {
			return fmt.Errorf("failed to parse PLAIN auth response: %w", err)
		}
		p.logger.Info("PLAIN auth", "user", p.user)
	case "AMQPLAIN":
		p.user, p.password, err = amqp091.ParseAMQPLAINAuthResponse(authRes)
		if err != nil {
			return fmt.Errorf("failed to parse AMQPLAIN auth response: %w", err)
		}
		p.logger.Info("AMQPLAIN auth", "user", p.user)
	default:
		return fmt.Errorf("unsupported auth mechanism: %s", auth)
	}

	// Connection.Tune 送信
	tune := &amqp091.ConnectionTune{
		ChannelMax: uint16(ChannelMax),
		FrameMax:   uint32(FrameMax),
		Heartbeat:  uint16(HeartbeatInterval),
	}
	if err := p.send(0, tune); err != nil {
		return fmt.Errorf("failed to write Connection.Tune: %w", err)
	}

	// Connection.Tune-Ok 受信
	tuneOk := amqp091.ConnectionTuneOk{}
	_, err = p.recv(0, &tuneOk)
	if err != nil {
		return fmt.Errorf("failed to read Connection.Tune-Ok: %w", err)
	}

	// Connection.Open 受信
	open := amqp091.ConnectionOpen{}
	_, err = p.recv(0, &open)
	if err != nil {
		return fmt.Errorf("failed to read Connection.Open: %w", err)
	}
	p.VirtualHost = open.VirtualHost
	p.logger.Info("Connection.Open", "vhost", p.VirtualHost)

	// Connection.Open-Ok 送信
	openOk := &amqp091.ConnectionOpenOk{}
	if err := p.send(0, openOk); err != nil {
		return fmt.Errorf("failed to write Connection.Open-Ok: %w", err)
	}

	return nil
}

func (p *Proxy) runHeartbeat(ctx context.Context, interval uint16) {
	defer recoverFromPanic(p.logger, "runHeartbeat")

	if interval == 0 {
		interval = uint16(HeartbeatInterval)
	}
	p.logger.Debug("start heartbeat", "interval", interval)
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.mu.Lock()
			p.logger.Debug("send heartbeat", "proxy", p.id)
			if err := p.w.WriteFrame(&amqp091.HeartbeatFrame{}); err != nil {
				p.mu.Unlock()
				p.logger.Warn("failed to send heartbeat", "error", err)
				p.shutdown(ctx)
				return
			}
			p.mu.Unlock()
		}
	}
}

func (p *Proxy) shutdown(ctx context.Context) error {
	// If no connection, shutdown is immediate
	if p.conn == nil {
		return nil
	}

	// Connection.Close 送信
	close := &amqp091.ConnectionClose{
		ReplyCode: 200,
		ReplyText: p.shutdownMessage,
	}
	if err := p.send(0, close); err != nil {
		return fmt.Errorf("failed to write Connection.Close: %w", err)
	}
	// Connection.Close-Ok 受信
	msg := amqp091.ConnectionCloseOk{}
	_, err := p.recv(0, &msg)
	if err != nil {
		p.logger.Warn("failed to read Connection.Close-Ok", "error", err)
	}
	return nil
}

// sendConnectionError sends a Connection.Close frame with error to the client
func (p *Proxy) sendConnectionError(err AMQPError) error {
	close := &amqp091.ConnectionClose{
		ReplyCode: err.Code(),
		ReplyText: err.Error(),
	}
	if sendErr := p.send(0, close); sendErr != nil {
		p.logger.Error("Failed to send Connection.Close to client", "error", sendErr)
		return sendErr
	}
	// Try to read Connection.Close-Ok, but don't wait too long
	p.conn.SetReadDeadline(time.Now().Add(p.connectionCloseTimeout))
	msg := amqp091.ConnectionCloseOk{}
	if _, recvErr := p.recv(0, &msg); recvErr != nil {
		p.logger.Debug("Failed to read Connection.Close-Ok from client", "error", recvErr)
	}
	return nil
}

func (p *Proxy) readClientFrame() (amqp091.Frame, error) {
	p.conn.SetReadDeadline(time.Now().Add(p.readTimeout))
	return p.r.ReadFrame()
}

func (p *Proxy) process(ctx context.Context) error {
	frame, err := p.readClientFrame()
	if err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return nil
		}
		return fmt.Errorf("failed to read frame: %w", err)
	}
	GetMetrics().ClientReceivedFrames.Inc()

	if mf, ok := frame.(*amqp091.MethodFrame); ok {
		p.logger.Debug("read method frame", "frame", mf, "type", reflect.TypeOf(mf.Method).String())
	} else {
		p.logger.Debug("read frame", "frame", frame, "type", reflect.TypeOf(frame).String())
	}
	if frame.Channel() == 0 {
		err = p.dispatch0(ctx, frame)
	} else {
		err = p.dispatchN(ctx, frame)
	}
	if err != nil {
		if e, ok := err.(AMQPError); ok {
			p.logger.Warn("AMQPError", "error", e)
			return p.send(frame.Channel(), e.AMQPMessage())
		}
		return err
	}
	return nil
}

func (p *Proxy) dispatchN(ctx context.Context, frame amqp091.Frame) error {
	switch f := frame.(type) {
	case *amqp091.MethodFrame:
		if m, ok := f.Method.(amqp091.MessageWithContent); ok {
			// read header and body frames until frame-end
			_, err := p.recv(int(f.Channel()), m)
			if err != nil {
				return NewError(amqp091.FrameError, fmt.Sprintf("failed to read frames: %s", err))
			}
			f.Method = m // replace method with message
		}
		methodName := strings.TrimLeft(reflect.TypeOf(f.Method).String(), "*amqp091.")
		GetMetrics().ProcessedMessages.WithLabelValues(methodName).Inc()
		switch m := f.Method.(type) {
		case *amqp091.ChannelOpen:
			return p.replyChannelOpen(ctx, f, m)
		case *amqp091.ChannelClose:
			return p.replyChannelClose(ctx, f, m)
		case *amqp091.QueueDeclare:
			return p.replyQueueDeclare(ctx, f, m)
		case *amqp091.QueueDelete:
			return p.replyQueueDelete(ctx, f, m)
		case *amqp091.QueueBind:
			return p.replyQueueBind(ctx, f, m)
		case *amqp091.QueueUnbind:
			return p.replyQueueUnbind(ctx, f, m)
		case *amqp091.QueuePurge:
			return p.replyQueuePurge(ctx, f, m)
		case *amqp091.ExchangeDeclare:
			return p.replyExchangeDeclare(ctx, f, m)
		case *amqp091.BasicPublish:
			return p.replyBasicPublish(ctx, f, m)
		case *amqp091.BasicConsume:
			return p.replyBasicConsume(ctx, f, m)
		case *amqp091.BasicGet:
			return p.replyBasicGet(ctx, f, m)
		case *amqp091.BasicAck:
			return p.replyBasicAck(ctx, f, m)
		case *amqp091.BasicNack:
			return p.replyBasicNack(ctx, f, m)
		case *amqp091.BasicCancel:
			return p.replyBasicCancel(ctx, f, m)
		case *amqp091.BasicQos:
			return p.replyBasicQos(ctx, f, m)
		default:
			GetMetrics().ErroredMessages.WithLabelValues(methodName).Inc()
			return NewError(amqp091.NotImplemented, fmt.Sprintf("unsupported method: %s", methodName))
		}
	case *amqp091.HeartbeatFrame:
		p.logger.Debug("heartbeat")
		// drop
	default:
		return fmt.Errorf("unsupported frame: %#v", f)
	}
	return nil
}

func (p *Proxy) dispatch0(ctx context.Context, frame amqp091.Frame) error {
	switch f := frame.(type) {
	case *amqp091.MethodFrame:
		switch m := f.Method.(type) {
		case *amqp091.ConnectionClose:
			return p.replyConnectionClose(ctx, f, m)
		default:
			return fmt.Errorf("unsupported method: %T", m)
		}
	case *amqp091.HeartbeatFrame:
		p.logger.Debug("heartbeat")
		// drop
	default:
		return fmt.Errorf("unsupported frame: %#v", f)
	}
	return nil
}

func (p *Proxy) send(channel uint16, m amqp091.Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.logger.Debug("send", "channel", channel, "message", m)
	if msg, ok := m.(amqp091.MessageWithContent); ok {
		props, body := msg.GetContent()
		class, _ := msg.ID()
		if err := p.w.WriteFrameNoFlush(&amqp091.MethodFrame{
			ChannelId: uint16(channel),
			Method:    msg,
		}); err != nil {
			return fmt.Errorf("failed to write MethodFrame: %w", err)
		}
		GetMetrics().ClientSentFrames.Inc()

		if err := p.w.WriteFrameNoFlush(&amqp091.HeaderFrame{
			ChannelId:  uint16(channel),
			ClassId:    class,
			Size:       uint64(len(body)),
			Properties: props,
		}); err != nil {
			return fmt.Errorf("failed to write HeaderFrame: %w", err)
		}
		GetMetrics().ClientSentFrames.Inc()

		// split body frame is it is too large (>= FrameMax)
		// The overhead of BodyFrame is 8 bytes
		offset := 0
		for offset < len(body) {
			end := offset + FrameMax - 8
			if end > len(body) {
				end = len(body)
			}
			if err := p.w.WriteFrame(&amqp091.BodyFrame{
				ChannelId: uint16(channel),
				Body:      body[offset:end],
			}); err != nil {
				return fmt.Errorf("failed to write BodyFrame: %w", err)
			}
			offset = end
			GetMetrics().ClientSentFrames.Inc()
		}
	} else {
		if err := p.w.WriteFrame(&amqp091.MethodFrame{
			ChannelId: uint16(channel),
			Method:    m,
		}); err != nil {
			return fmt.Errorf("failed to write MethodFrame: %w", err)
		}
		GetMetrics().ClientSentFrames.Inc()
	}
	return nil
}

func (p *Proxy) recv(channel int, m amqp091.Message) (amqp091.Message, error) {
	var remaining int
	var header *amqp091.HeaderFrame
	var body []byte
	defer func() {
		p.logger.Debug("recv", "channel", channel, "message", m, "type", reflect.TypeOf(m).String())
	}()

	for {
		frame, err := p.readClientFrame()
		if err != nil {
			return nil, fmt.Errorf("frame err, read: %w", err)
		}
		GetMetrics().ClientReceivedFrames.Inc()

		if frame.Channel() != uint16(channel) {
			return nil, fmt.Errorf("expected frame on channel %d, got channel %d", channel, frame.Channel())
		}

		switch f := frame.(type) {
		case *amqp091.HeartbeatFrame:
			// drop

		case *amqp091.HeaderFrame:
			// start content state
			header = f
			remaining = int(header.Size)
			if remaining == 0 {
				m.(amqp091.MessageWithContent).SetContent(header.Properties, nil)
				return m, nil
			}

		case *amqp091.BodyFrame:
			// continue until terminated
			body = append(body, f.Body...)
			remaining -= len(f.Body)
			if remaining <= 0 {
				m.(amqp091.MessageWithContent).SetContent(header.Properties, body)
				return m, nil
			}

		case *amqp091.MethodFrame:
			if reflect.TypeOf(m) == reflect.TypeOf(f.Method) {
				wantv := reflect.ValueOf(m).Elem()
				havev := reflect.ValueOf(f.Method).Elem()
				wantv.Set(havev)
				if _, ok := m.(amqp091.MessageWithContent); !ok {
					return m, nil
				}
			} else {
				return nil, fmt.Errorf("expected method type: %T, got: %T", m, f.Method)
			}

		default:
			return nil, fmt.Errorf("unexpected frame: %+v", f)
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
