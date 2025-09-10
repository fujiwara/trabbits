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
	dto "github.com/prometheus/client_model/go"
	rabbitmq "github.com/rabbitmq/amqp091-go"
)

var (
	ChannelMax             = 1023
	HeartbeatInterval      = 60
	FrameMax               = 128 * 1024
	UpstreamDefaultTimeout = 5 * time.Second
)

var Debug bool

// healthManagers stores health check managers for cluster upstreams
var healthManagers = sync.Map{} // upstream name -> *NodeHealthManager

func run(ctx context.Context, opt *CLI) error {
	cfg, err := LoadConfig(ctx, opt.Config)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	storeConfig(cfg)

	// Write PID file if specified
	if opt.Run.PidFile != "" {
		if err := writePidFile(opt.Run.PidFile); err != nil {
			return fmt.Errorf("failed to write PID file: %w", err)
		}
		defer removePidFile(opt.Run.PidFile)
	}

	// Initialize health check managers for cluster upstreams
	if err := initHealthManagers(ctx, cfg); err != nil {
		return fmt.Errorf("failed to init health managers: %w", err)
	}

	cancelAPI, err := runAPIServer(ctx, opt)
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
				if _, err := reloadConfigFromFile(ctx, opt.Config); err != nil {
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

	return boot(ctx, listener)
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

func boot(ctx context.Context, listener net.Listener) error {
	port := listener.Addr().(*net.TCPAddr).Port
	slog.Info("trabbits started", "port", port)
	go func() {
		<-ctx.Done()
		if err := listener.Close(); err != nil {
			slog.Error("Failed to close listener", "error", err)
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
		go handleConnection(ctx, conn)
	}
}

// initHealthManagers initializes health check managers for cluster upstreams
func initHealthManagers(ctx context.Context, cfg *Config) error {
	// Stop existing health managers
	healthManagers.Range(func(key, value interface{}) bool {
		if mgr, ok := value.(*NodeHealthManager); ok {
			mgr.Stop()
		}
		healthManagers.Delete(key)
		return true
	})

	// Create new health managers for cluster upstreams
	for _, upstream := range cfg.Upstreams {
		if upstream.Cluster != nil && upstream.HealthCheck != nil {
			mgr := NewNodeHealthManager(upstream)
			if mgr != nil {
				mgr.StartHealthCheck(ctx)
				healthManagers.Store(upstream.Name, mgr)
				slog.Info("Health check manager initialized",
					"upstream", upstream.Name,
					"nodes", len(upstream.Cluster.Nodes))
			}
		}
	}

	return nil
}

var amqpHeader = []byte{'A', 'M', 'Q', 'P', 0, 0, 9, 1}

func handleConnection(ctx context.Context, conn net.Conn) {
	metrics.ClientConnections.Inc()
	metrics.ClientTotalConnections.Inc()
	defer func() {
		conn.Close()
		metrics.ClientConnections.Dec()
	}()

	slog.Info("new connection", "client_addr", conn.RemoteAddr().String())
	// AMQP プロトコルヘッダー受信
	header := make([]byte, 8)
	if _, err := io.ReadFull(conn, header); err != nil {
		slog.Warn("Failed to read AMQP header:", "error", err)
		metrics.ClientConnectionErrors.Inc()
		return
	}
	if !bytes.Equal(header, amqpHeader) {
		slog.Warn("Invalid AMQP protocol header", "header", header)
		metrics.ClientConnectionErrors.Inc()
		return
	}

	s := NewProxy(conn)
	defer s.Close()
	s.logger.Info("proxy created")

	if err := s.handshake(ctx); err != nil {
		s.logger.Warn("Failed to handshake", "error", err)
		metrics.ClientConnectionErrors.Inc()
		return
	}
	s.logger.Info("handshake completed")

	// subCtx is used for client connection depends on parent context
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	cfg := mustGetConfig()
	if err := s.ConnectToUpstreams(subCtx, cfg.Upstreams, s.clientProps); err != nil {
		s.logger.Warn("Failed to connect to upstreams", "error", err)
		return
	}
	s.logger.Info("connected to upstreams")

	go s.runHeartbeat(subCtx, uint16(HeartbeatInterval))

	// wait for client frames
	for {
		select {
		case <-ctx.Done(): // parent (server) context is done
			s.shutdown(subCtx) // graceful shutdown
			return             // subCtx will be canceled by defer ^
		case <-subCtx.Done():
			return
		case upstreamName := <-s.upstreamDisconnect:
			// An upstream connection was lost
			s.logger.Error("Upstream connection lost, closing client connection",
				"upstream", upstreamName)
			// Send Connection.Close to client with connection-forced error
			s.sendConnectionError(NewError(amqp091.ConnectionForced,
				fmt.Sprintf("Upstream connection lost: %s", upstreamName)))
			return
		default:
		}
		if err := s.process(subCtx); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) || isBrokenPipe(err) {
				s.logger.Info("closed connection")
			} else {
				s.logger.Warn("failed to process", "error", err)
			}
			break
		}
	}
}

func (s *Proxy) connectToUpstreamServer(addr string, props amqp091.Table, timeout time.Duration) (*rabbitmq.Connection, error) {
	u := &url.URL{
		Scheme: "amqp",
		User:   url.UserPassword(s.user, s.password),
		Host:   addr,
		Path:   s.VirtualHost,
	}
	s.logger.Info("connect to upstream", "url", safeURLString(*u))
	// Copy all client properties and override specific ones
	upstreamProps := rabbitmq.Table{}
	for k, v := range props {
		upstreamProps[k] = v
	}
	// overwrite product property to include trabbits and client address
	upstreamProps["product"] = fmt.Sprintf("%s (%s) via trabbits/%s", props["product"], s.ClientAddr(), Version)

	cfg := rabbitmq.Config{
		Properties: upstreamProps,
		Dial:       rabbitmq.DefaultDial(timeout),
	}
	conn, err := rabbitmq.DialConfig(u.String(), cfg)
	if err != nil {
		metrics.UpstreamConnectionErrors.WithLabelValues(addr).Inc()
		return nil, fmt.Errorf("failed to open upstream %s %w", u, err)
	}
	return conn, nil
}

func (s *Proxy) connectToUpstreamServers(upstreamName string, addrs []string, props amqp091.Table, timeout time.Duration) (*rabbitmq.Connection, string, error) {
	var nodesToTry []string

	// Use health manager if available for cluster upstreams
	if mgr, ok := healthManagers.Load(upstreamName); ok {
		if healthMgr := mgr.(*NodeHealthManager); healthMgr != nil {
			nodesToTry = healthMgr.GetHealthyNodes()
			s.logger.Debug("Using health-based node selection",
				"upstream", upstreamName,
				"healthy_nodes", len(nodesToTry),
				"total_nodes", len(addrs))
		}
	}

	// Fall back to all nodes if no health manager or no healthy nodes
	if len(nodesToTry) == 0 {
		nodesToTry = addrs
	}

	// Sort nodes using least connection algorithm
	nodesToTry = sortNodesByLeastConnections(nodesToTry)

	// Try to connect to each node
	for _, addr := range nodesToTry {
		conn, err := s.connectToUpstreamServer(addr, props, timeout)
		if err == nil {
			s.logger.Info("Connected to upstream node", "upstream", upstreamName, "address", addr)
			return conn, addr, nil
		} else {
			s.logger.Warn("Failed to connect to upstream node",
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
		gauge := metrics.UpstreamConnections.WithLabelValues(addr)
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

func (s *Proxy) ConnectToUpstreams(ctx context.Context, upstreamConfigs []UpstreamConfig, props amqp091.Table) error {
	for _, c := range upstreamConfigs {
		timeout := c.Timeout.ToDuration()
		if timeout == 0 {
			timeout = UpstreamDefaultTimeout
		}
		conn, addr, err := s.connectToUpstreamServers(c.Name, c.Addresses(), props, timeout)
		if err != nil {
			return err
		}
		us := NewUpstream(conn, s.logger, c, addr)
		s.upstreams = append(s.upstreams, us)

		// Start monitoring the upstream connection
		go s.MonitorUpstreamConnection(ctx, us)
	}
	return nil
}

func (s *Proxy) handshake(ctx context.Context) error {
	// Connection.Start 送信
	start := &amqp091.ConnectionStart{
		VersionMajor: 0,
		VersionMinor: 9,
		Mechanisms:   "PLAIN",
		Locales:      "en_US",
	}
	if err := s.send(0, start); err != nil {
		return fmt.Errorf("failed to write Connection.Start: %w", err)
	}

	// Connection.Start-Ok 受信（認証情報含む）
	startOk := amqp091.ConnectionStartOk{}
	_, err := s.recv(0, &startOk)
	if err != nil {
		return fmt.Errorf("failed to read Connection.Start-Ok: %w", err)
	}
	s.clientProps = startOk.ClientProperties
	s.logger = s.logger.With("client", s.ClientBanner())
	auth := startOk.Mechanism
	authRes := startOk.Response
	switch auth {
	case "PLAIN":
		s.user, s.password, err = amqp091.ParsePLAINAuthResponse(authRes)
		if err != nil {
			return fmt.Errorf("failed to parse PLAIN auth response: %w", err)
		}
		s.logger.Info("PLAIN auth", "user", s.user)
	case "AMQPLAIN":
		s.user, s.password, err = amqp091.ParseAMQPLAINAuthResponse(authRes)
		if err != nil {
			return fmt.Errorf("failed to parse AMQPLAIN auth response: %w", err)
		}
		s.logger.Info("AMQPLAIN auth", "user", s.user)
	default:
		return fmt.Errorf("unsupported auth mechanism: %s", auth)
	}

	// Connection.Tune 送信
	tune := &amqp091.ConnectionTune{
		ChannelMax: uint16(ChannelMax),
		FrameMax:   uint32(FrameMax),
		Heartbeat:  uint16(HeartbeatInterval),
	}
	if err := s.send(0, tune); err != nil {
		return fmt.Errorf("failed to write Connection.Tune: %w", err)
	}

	// Connection.Tune-Ok 受信
	tuneOk := amqp091.ConnectionTuneOk{}
	_, err = s.recv(0, &tuneOk)
	if err != nil {
		return fmt.Errorf("failed to read Connection.Tune-Ok: %w", err)
	}

	// Connection.Open 受信
	open := amqp091.ConnectionOpen{}
	_, err = s.recv(0, &open)
	if err != nil {
		return fmt.Errorf("failed to read Connection.Open: %w", err)
	}
	s.VirtualHost = open.VirtualHost
	s.logger.Info("Connection.Open", "vhost", s.VirtualHost)

	// Connection.Open-Ok 送信
	openOk := &amqp091.ConnectionOpenOk{}
	if err := s.send(0, openOk); err != nil {
		return fmt.Errorf("failed to write Connection.Open-Ok: %w", err)
	}

	return nil
}

func (s *Proxy) runHeartbeat(ctx context.Context, interval uint16) {
	if interval == 0 {
		interval = uint16(HeartbeatInterval)
	}
	s.logger.Debug("start heartbeat", "interval", interval)
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			s.logger.Debug("send heartbeat", "proxy", s.id)
			if err := s.w.WriteFrame(&amqp091.HeartbeatFrame{}); err != nil {
				s.mu.Unlock()
				s.logger.Warn("failed to send heartbeat", "error", err)
				s.shutdown(ctx)
				return
			}
			s.mu.Unlock()
		}
	}
}

func (s *Proxy) shutdown(ctx context.Context) error {
	// Connection.Close 送信
	close := &amqp091.ConnectionClose{
		ReplyCode: 200,
		ReplyText: "Goodbye",
	}
	if err := s.send(0, close); err != nil {
		return fmt.Errorf("failed to write Connection.Close: %w", err)
	}
	// Connection.Close-Ok 受信
	msg := amqp091.ConnectionCloseOk{}
	_, err := s.recv(0, &msg)
	if err != nil {
		s.logger.Warn("failed to read Connection.Close-Ok", "error", err)
	}
	return nil
}

// sendConnectionError sends a Connection.Close frame with error to the client
func (s *Proxy) sendConnectionError(err AMQPError) error {
	close := &amqp091.ConnectionClose{
		ReplyCode: err.Code(),
		ReplyText: err.Error(),
	}
	if sendErr := s.send(0, close); sendErr != nil {
		s.logger.Error("Failed to send Connection.Close to client", "error", sendErr)
		return sendErr
	}
	// Try to read Connection.Close-Ok, but don't wait too long
	s.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	msg := amqp091.ConnectionCloseOk{}
	if _, recvErr := s.recv(0, &msg); recvErr != nil {
		s.logger.Debug("Failed to read Connection.Close-Ok from client", "error", recvErr)
	}
	return nil
}

var readTimeout = 5 * time.Second

func (s *Proxy) readClientFrame() (amqp091.Frame, error) {
	s.conn.SetReadDeadline(time.Now().Add(readTimeout))
	return s.r.ReadFrame()
}

func (s *Proxy) process(ctx context.Context) error {
	frame, err := s.readClientFrame()
	if err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return nil
		}
		return fmt.Errorf("failed to read frame: %w", err)
	}
	metrics.ClientReceivedFrames.Inc()

	if mf, ok := frame.(*amqp091.MethodFrame); ok {
		s.logger.Debug("read method frame", "frame", mf, "type", reflect.TypeOf(mf.Method).String())
	} else {
		s.logger.Debug("read frame", "frame", frame, "type", reflect.TypeOf(frame).String())
	}
	if frame.Channel() == 0 {
		err = s.dispatch0(ctx, frame)
	} else {
		err = s.dispatchN(ctx, frame)
	}
	if err != nil {
		if e, ok := err.(AMQPError); ok {
			s.logger.Warn("AMQPError", "error", e)
			return s.send(frame.Channel(), e.AMQPMessage())
		}
		return err
	}
	return nil
}

func (s *Proxy) dispatchN(ctx context.Context, frame amqp091.Frame) error {
	switch f := frame.(type) {
	case *amqp091.MethodFrame:
		if m, ok := f.Method.(amqp091.MessageWithContent); ok {
			// read header and body frames until frame-end
			_, err := s.recv(int(f.Channel()), m)
			if err != nil {
				return NewError(amqp091.FrameError, fmt.Sprintf("failed to read frames: %s", err))
			}
			f.Method = m // replace method with message
		}
		methodName := strings.TrimLeft(reflect.TypeOf(f.Method).String(), "*amqp091.")
		metrics.ProcessedMessages.WithLabelValues(methodName).Inc()
		switch m := f.Method.(type) {
		case *amqp091.ChannelOpen:
			return s.replyChannelOpen(ctx, f, m)
		case *amqp091.ChannelClose:
			return s.replyChannelClose(ctx, f, m)
		case *amqp091.QueueDeclare:
			return s.replyQueueDeclare(ctx, f, m)
		case *amqp091.QueueDelete:
			return s.replyQueueDelete(ctx, f, m)
		case *amqp091.QueueBind:
			return s.replyQueueBind(ctx, f, m)
		case *amqp091.QueueUnbind:
			return s.replyQueueUnbind(ctx, f, m)
		case *amqp091.QueuePurge:
			return s.replyQueuePurge(ctx, f, m)
		case *amqp091.ExchangeDeclare:
			return s.replyExchangeDeclare(ctx, f, m)
		case *amqp091.BasicPublish:
			return s.replyBasicPublish(ctx, f, m)
		case *amqp091.BasicConsume:
			return s.replyBasicConsume(ctx, f, m)
		case *amqp091.BasicGet:
			return s.replyBasicGet(ctx, f, m)
		case *amqp091.BasicAck:
			return s.replyBasicAck(ctx, f, m)
		case *amqp091.BasicNack:
			return s.replyBasicNack(ctx, f, m)
		case *amqp091.BasicCancel:
			return s.replyBasicCancel(ctx, f, m)
		case *amqp091.BasicQos:
			return s.replyBasicQos(ctx, f, m)
		default:
			metrics.ErroredMessages.WithLabelValues(methodName).Inc()
			return NewError(amqp091.NotImplemented, fmt.Sprintf("unsupported method: %s", methodName))
		}
	case *amqp091.HeartbeatFrame:
		s.logger.Debug("heartbeat")
		// drop
	default:
		return fmt.Errorf("unsupported frame: %#v", f)
	}
	return nil
}

func (s *Proxy) dispatch0(ctx context.Context, frame amqp091.Frame) error {
	switch f := frame.(type) {
	case *amqp091.MethodFrame:
		switch m := f.Method.(type) {
		case *amqp091.ConnectionClose:
			return s.replyConnectionClose(ctx, f, m)
		default:
			return fmt.Errorf("unsupported method: %T", m)
		}
	case *amqp091.HeartbeatFrame:
		s.logger.Debug("heartbeat")
		// drop
	default:
		return fmt.Errorf("unsupported frame: %#v", f)
	}
	return nil
}

func (s *Proxy) send(channel uint16, m amqp091.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logger.Debug("send", "channel", channel, "message", m)
	if msg, ok := m.(amqp091.MessageWithContent); ok {
		props, body := msg.GetContent()
		class, _ := msg.ID()
		if err := s.w.WriteFrameNoFlush(&amqp091.MethodFrame{
			ChannelId: uint16(channel),
			Method:    msg,
		}); err != nil {
			return fmt.Errorf("failed to write MethodFrame: %w", err)
		}
		metrics.ClientSentFrames.Inc()

		if err := s.w.WriteFrameNoFlush(&amqp091.HeaderFrame{
			ChannelId:  uint16(channel),
			ClassId:    class,
			Size:       uint64(len(body)),
			Properties: props,
		}); err != nil {
			return fmt.Errorf("failed to write HeaderFrame: %w", err)
		}
		metrics.ClientSentFrames.Inc()

		// split body frame is it is too large (>= FrameMax)
		// The overhead of BodyFrame is 8 bytes
		offset := 0
		for offset < len(body) {
			end := offset + FrameMax - 8
			if end > len(body) {
				end = len(body)
			}
			if err := s.w.WriteFrame(&amqp091.BodyFrame{
				ChannelId: uint16(channel),
				Body:      body[offset:end],
			}); err != nil {
				return fmt.Errorf("failed to write BodyFrame: %w", err)
			}
			offset = end
			metrics.ClientSentFrames.Inc()
		}
	} else {
		if err := s.w.WriteFrame(&amqp091.MethodFrame{
			ChannelId: uint16(channel),
			Method:    m,
		}); err != nil {
			return fmt.Errorf("failed to write MethodFrame: %w", err)
		}
		metrics.ClientSentFrames.Inc()
	}
	return nil
}

func (s *Proxy) recv(channel int, m amqp091.Message) (amqp091.Message, error) {
	var remaining int
	var header *amqp091.HeaderFrame
	var body []byte
	defer func() {
		s.logger.Debug("recv", "channel", channel, "message", m, "type", reflect.TypeOf(m).String())
	}()

	for {
		frame, err := s.readClientFrame()
		if err != nil {
			return nil, fmt.Errorf("frame err, read: %w", err)
		}
		metrics.ClientReceivedFrames.Inc()

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
