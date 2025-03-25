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
	"net/url"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/fujiwara/trabbits/amqp091"

	rabbitmq "github.com/rabbitmq/amqp091-go"
)

var (
	ChannelMax        = 1023
	HeartbeatInterval = 60
	FrameMax          = 128 * 1024
)

var Debug bool

func run(ctx context.Context, opt *CLI) error {
	cfg, err := LoadConfig(opt.Config)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	storeConfig(cfg)

	slog.Info("trabbits starting", "version", Version, "port", opt.Port)
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", opt.Port))
	if err != nil {
		return fmt.Errorf("failed to start AMQP server: %w", err)
	}
	defer listener.Close()

	cancel, err := runAPIServer(ctx, opt)
	if err != nil {
		return fmt.Errorf("failed to start API server: %w", err)
	}
	defer cancel()

	return boot(ctx, listener)
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
		slog.Warn("Failed to handshake", "error", err)
		metrics.ClientConnectionErrors.Inc()
		return
	}

	cfg := mustGetConfig()
	if err := s.ConnectToUpstreams(ctx, cfg.Upstreams, s.clientProps); err != nil {
		slog.Warn("Failed to connect to upstreams", "error", err)
		return
	}
	s.logger.Info("connected to upstream")

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go s.runHeartbeat(ctx, uint16(HeartbeatInterval))

	s.logger.Info("handshake completed")
	// ここからクライアントのリクエストを待ち受ける
	for {
		select {
		case <-ctx.Done():
			s.shutdown(ctx)
			return
		default:
		}
		if err := s.process(ctx); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) || isBrokenPipe(err) {
				s.logger.Info("closed connection")
			} else {
				s.logger.Warn("failed to process", "error", err)
			}
			break
		}
	}
}

func (s *Proxy) ConnectToUpstreams(_ context.Context, upstreamConfigs []UpstreamConfig, props amqp091.Table) error {
	for _, c := range upstreamConfigs {
		addr := net.JoinHostPort(c.Host, fmt.Sprintf("%d", c.Port))
		u := &url.URL{
			Scheme: "amqp",
			User:   url.UserPassword(s.user, s.password),
			Host:   addr,
			Path:   s.VirtualHost,
		}
		s.logger.Info("connect to upstream", "url", safeURLString(*u))
		cfg := rabbitmq.Config{
			Properties: rabbitmq.Table{
				"version":  props["version"],
				"platform": props["platform"],
				"product":  fmt.Sprintf("%s (%s) via trabbits/%s", props["product"], s.ClientAddr(), Version),
			},
		}
		conn, err := rabbitmq.DialConfig(u.String(), cfg)
		if err != nil {
			metrics.UpstreamConnectionErrors.WithLabelValues(addr).Inc()
			return fmt.Errorf("failed to open upstream %s %w", u, err)
		}
		us := NewUpstream(addr, conn, s.logger, c)
		s.upstreams = append(s.upstreams, us)
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

func (s *Proxy) process(ctx context.Context) error {
	frame, err := s.r.ReadFrame()
	if err != nil {
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
		frame, err := s.r.ReadFrame()
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
