// MIT License
// Copyright (c) 2025 FUJIWARA Shunichiro

package trabbits

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/url"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/fujiwara/trabbits/amqp091"
	"github.com/fujiwara/trabbits/config"
	metricsstore "github.com/fujiwara/trabbits/metrics"
	"github.com/fujiwara/trabbits/pattern"
	dto "github.com/prometheus/client_model/go"
	rabbitmq "github.com/rabbitmq/amqp091-go"
)

type Proxy struct {
	VirtualHost string

	id   string
	conn net.Conn
	r    *amqp091.Reader // framer <- client
	w    *amqp091.Writer // framer -> client

	mu sync.Mutex

	upstreams []*Upstream

	logger                 *slog.Logger
	user                   string
	password               string
	clientProps            amqp091.Table
	readTimeout            time.Duration
	connectionCloseTimeout time.Duration

	configHash         string                // hash of config used for this proxy
	upstreamDisconnect chan string           // channel to notify upstream disconnection
	shutdownMessage    string                // message to send when shutting down
	connectedAt        time.Time             // timestamp when the client connected
	stats              *ProxyStats           // statistics for this proxy
	probeChan          chan probeLog         // channel to send probe logs
	metrics            *metricsstore.Metrics // metrics instance for this proxy
}

func (p *Proxy) Upstreams() []*Upstream {
	return p.upstreams
}

// GetProbeChan returns the probe channel for external access
func (p *Proxy) GetProbeChan() chan probeLog {
	return p.probeChan
}

// sendProbeLog sends a probe log message with structured attributes to the probe channel
// If the channel is full, it removes the oldest log and sends the new one
func (p *Proxy) sendProbeLog(message string, attrs ...any) {
	if p.probeChan == nil {
		return
	}

	newLog := probeLog{
		Timestamp: time.Now(),
		Message:   message,
		attrs:     attrs, // Store as slice without conversion
	}

	select {
	case p.probeChan <- newLog:
		// Successfully sent
	default:
		// Channel is full, discard one old log and try to send the new one
		select {
		case <-p.probeChan: // Remove oldest log
		default:
		}
		// Try to send new log, but don't block if still full (race condition with other goroutines)
		select {
		case p.probeChan <- newLog:
		default:
			// Still full, drop this log
		}
	}
}

func (p *Proxy) Upstream(i int) *Upstream {
	return p.upstreams[i]
}

func (p *Proxy) GetChannels(id uint16) ([]*rabbitmq.Channel, error) {
	var chs []*rabbitmq.Channel
	for _, us := range p.upstreams {
		ch, err := us.GetChannel(id)
		if err != nil {
			return nil, err
		}
		chs = append(chs, ch)
	}
	return chs, nil
}

func (p *Proxy) GetChannel(id uint16, routingKey string) (*rabbitmq.Channel, error) {
	var routed *Upstream
	for _, us := range p.upstreams {
		for _, keyPattern := range us.keyPatterns {
			if pattern.Match(routingKey, keyPattern) {
				routed = us
				p.sendProbeLog("matched pattern", "pattern", keyPattern, "routing_key", routingKey)
				break
			}
		}
	}
	if routed != nil {
		return routed.GetChannel(id)
	}
	us := p.upstreams[0] // default upstream
	p.sendProbeLog("not matched any patterns, using default upstream", "routing_key", routingKey)
	return us.GetChannel(id)
}

func (p *Proxy) ID() string {
	return p.id
}

func (p *Proxy) ClientAddr() string {
	if p.conn == nil {
		return ""
	}
	if tcpConn, ok := p.conn.(*net.TCPConn); ok {
		return tcpConn.RemoteAddr().String()
	}
	return ""
}

func (p *Proxy) Close() {
	for _, us := range p.Upstreams() {
		if err := us.Close(); err != nil {
			us.logger.Warn("failed to close upstream", "error", err)
		}
	}
}

func (p *Proxy) NewChannel(id uint16) error {
	for _, us := range p.Upstreams() {
		if _, err := us.NewChannel(id); err != nil {
			return fmt.Errorf("failed to create channel: %w", err)
		}
	}
	return nil
}

func (p *Proxy) CloseChannel(id uint16) error {
	for _, us := range p.Upstreams() {
		if err := us.CloseChannel(id); err != nil {
			return err
		}
	}
	return nil
}

func (p *Proxy) ClientBanner() string {
	if p.clientProps == nil {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s", p.clientProps["platform"], p.clientProps["product"], p.clientProps["version"])
}

// MonitorUpstreamConnection monitors an upstream connection and notifies when it closes
func (p *Proxy) MonitorUpstreamConnection(ctx context.Context, upstream *Upstream) {
	defer recoverFromPanic(p.logger, "MonitorUpstreamConnection", p.metrics)

	select {
	case <-ctx.Done():
		p.sendProbeLog("Upstream monitoring stopped by context", "upstream", upstream.String())
		return
	case err := <-upstream.NotifyClose():
		if err != nil {
			p.logger.Warn("Upstream connection closed with error",
				"upstream", upstream.String(),
				"address", upstream.address,
				"error", err)
		} else {
			p.logger.Warn("Upstream connection closed gracefully",
				"upstream", upstream.String(),
				"address", upstream.address)
		}
		// Notify proxy about the disconnection
		select {
		case p.upstreamDisconnect <- upstream.String():
		default:
			// Channel is full, log and continue
			p.logger.Error("Failed to notify upstream disconnection - channel full",
				"upstream", upstream.String())
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
		p.metrics.UpstreamConnectionErrors.WithLabelValues(addr).Inc()
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
	nodesToTry = p.sortNodesByLeastConnections(nodesToTry)

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
func (p *Proxy) sortNodesByLeastConnections(nodes []string) []string {
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
		gauge := p.metrics.UpstreamConnections.WithLabelValues(addr)
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
		us := NewUpstream(conn, p.logger, c, addr, p.metrics)
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
	defer recoverFromPanic(p.logger, "runHeartbeat", p.metrics)

	if interval == 0 {
		interval = uint16(HeartbeatInterval)
	}
	p.sendProbeLog("start heartbeat", "interval", interval)
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.mu.Lock()
			p.sendProbeLog("send heartbeat", "proxy", p.id)
			if err := p.w.WriteFrame(&amqp091.HeartbeatFrame{}); err != nil {
				p.mu.Unlock()
				p.logger.Warn("failed to send heartbeat", "error", err)
				p.shutdown(ctx)
				return
			}
			// Count heartbeat frame
			if p.stats != nil {
				p.stats.IncrementSentFrames()
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
		p.sendProbeLog("Failed to read Connection.Close-Ok from client", "error", recvErr)
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

	// Update frame statistics
	if p.stats != nil {
		p.stats.IncrementReceivedFrames()
	}
	p.metrics.ClientReceivedFrames.Inc()

	if mf, ok := frame.(*amqp091.MethodFrame); ok {
		p.sendProbeLog("read method frame", "type", amqp091.TypeName(mf.Method))
	} else {
		p.sendProbeLog("read frame", "type", amqp091.TypeName(frame))
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
		methodName := amqp091.TypeName(f.Method)
		p.metrics.ProcessedMessages.WithLabelValues(methodName).Inc()
		// Update proxy-specific statistics
		if p.stats != nil {
			p.stats.IncrementMethod(methodName)
		}
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
			p.metrics.ErroredMessages.WithLabelValues(methodName).Inc()
			return NewError(amqp091.NotImplemented, fmt.Sprintf("unsupported method: %s", methodName))
		}
	case *amqp091.HeartbeatFrame:
		p.sendProbeLog("received heartbeat from client")
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
		p.sendProbeLog("received heartbeat from server")
		// drop
	default:
		return fmt.Errorf("unsupported frame: %#v", f)
	}
	return nil
}

func (p *Proxy) send(channel uint16, m amqp091.Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sendProbeLog("send", "channel", channel, "type", amqp091.TypeName(m))
	if msg, ok := m.(amqp091.MessageWithContent); ok {
		props, body := msg.GetContent()
		class, _ := msg.ID()
		if err := p.w.WriteFrameNoFlush(&amqp091.MethodFrame{
			ChannelId: uint16(channel),
			Method:    msg,
		}); err != nil {
			return fmt.Errorf("failed to write MethodFrame: %w", err)
		}
		p.metrics.ClientSentFrames.Inc()
		if p.stats != nil {
			p.stats.IncrementSentFrames()
		}

		if err := p.w.WriteFrameNoFlush(&amqp091.HeaderFrame{
			ChannelId:  uint16(channel),
			ClassId:    class,
			Size:       uint64(len(body)),
			Properties: props,
		}); err != nil {
			return fmt.Errorf("failed to write HeaderFrame: %w", err)
		}
		p.metrics.ClientSentFrames.Inc()
		if p.stats != nil {
			p.stats.IncrementSentFrames()
		}

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
			p.metrics.ClientSentFrames.Inc()
			if p.stats != nil {
				p.stats.IncrementSentFrames()
			}
		}
	} else {
		if err := p.w.WriteFrame(&amqp091.MethodFrame{
			ChannelId: uint16(channel),
			Method:    m,
		}); err != nil {
			return fmt.Errorf("failed to write MethodFrame: %w", err)
		}
		p.metrics.ClientSentFrames.Inc()
		if p.stats != nil {
			p.stats.IncrementSentFrames()
		}
	}
	return nil
}

func (p *Proxy) recv(channel int, m amqp091.Message) (amqp091.Message, error) {
	var remaining int
	var header *amqp091.HeaderFrame
	var body []byte
	defer func() {
		p.sendProbeLog("recv", "channel", channel, "type", amqp091.TypeName(m))
	}()

	for {
		frame, err := p.readClientFrame()
		if err != nil {
			return nil, fmt.Errorf("frame err, read: %w", err)
		}
		p.metrics.ClientReceivedFrames.Inc()

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
