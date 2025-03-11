// Copyright (c) 2021 VMware, Inc. or its affiliates. All Rights Reserved.
// Copyright (c) 2012-2021, Sean Treadway, SoundCloud Ltd.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package trabbits

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"reflect"
	"strings"

	"github.com/fujiwara/trabbits/amqp091"
	"github.com/google/uuid"

	rabbitmq "github.com/rabbitmq/amqp091-go"
)

func init() {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(handler))
}

const (
	ChannelMax        = 1023
	HeartbeatInterval = 60
	FrameMax          = 128 * 1024
)

var Debug = true

var config = &Config{
	Upstream: UpstreamConfig{
		"amqp://127.0.0.1:5672/",
	},
}

func Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", ":5673")
	if err != nil {
		return fmt.Errorf("failed to start AMQP server: %w", err)
	}
	defer listener.Close()
	return bootProxy(ctx, listener)
}

func bootProxy(ctx context.Context, listener net.Listener) error {
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
	defer conn.Close()

	slog.Info("new connection", "addr", conn.RemoteAddr())
	// AMQP プロトコルヘッダー受信
	header := make([]byte, 8)
	if _, err := io.ReadFull(conn, header); err != nil {
		slog.Warn("Failed to read AMQP header:", "error", err)
		return
	}
	if !bytes.Equal(header, amqpHeader) {
		slog.Warn("Invalid AMQP protocol header", "header", header)
		return
	}

	s := NewProxy(conn)
	slog.Info("proxy created", "proxy", s.id, "addr", conn.RemoteAddr())
	client, err := s.handshake(ctx)
	if err != nil {
		slog.Warn("Failed to handshake", "error", err)
		return
	}
	slog.Info("handshake done", "proxy", s.id, "client", client.id, "user", client.user, "addr", conn.RemoteAddr())
	// ここからクライアントのリクエストを待ち受ける
	for {
		select {
		case <-ctx.Done():
			slog.Info("context done")
			s.shutdown(ctx)
			return
		default:
		}
		if err := s.process(ctx, client); err != nil {
			slog.Warn("failed to process", "error", err)
			if err := s.upstream.Close(); err != nil {
				slog.Warn("failed to close upstream", "error", err)
			}
			break
		}
	}
	slog.Info("goodbye", "addr", conn.RemoteAddr())
}

type Proxy struct {
	id string
	r  *amqp091.Reader // framer <- client
	w  *amqp091.Writer // framer -> client

	upstream *rabbitmq.Connection
}

func (s *Proxy) ConnectToUpstream(addr string, user, pass string) error {
	slog.Info("connect to upstream", "addr", addr, "proxy", s.id)
	u, err := url.Parse(addr)
	if err != nil {
		return fmt.Errorf("failed to parse upstream address %s %w", addr, err)
	}
	if u.Scheme != "amqp" {
		return fmt.Errorf("unsupported upstream scheme %s %s", addr, u.Scheme)
	}
	u.User = url.UserPassword(user, pass)

	conn, err := rabbitmq.Dial(u.String())
	if err != nil {
		return fmt.Errorf("failed to connect to upstream %s %w", addr, err)
	}
	s.upstream = conn
	return nil
}

func (s *Proxy) handshake(ctx context.Context) (*Client, error) {
	client := NewClient(nil)

	// Connection.Start 送信
	start := &amqp091.ConnectionStart{
		VersionMajor: 0,
		VersionMinor: 9,
		Mechanisms:   "PLAIN",
		Locales:      "en_US",
	}
	if err := s.send(0, start); err != nil {
		return nil, fmt.Errorf("failed to write Connection.Start: %w", err)
	}

	// Connection.Start-Ok 受信（認証情報含む）
	startOk := amqp091.ConnectionStartOk{}
	_, err := s.recv(0, &startOk)
	if err != nil {
		return nil, fmt.Errorf("failed to read Connection.Start-Ok: %w", err)
	}
	auth := startOk.Mechanism
	if auth != "PLAIN" {
		return nil, fmt.Errorf("unsupported auth mechanism: %s", auth)
	}
	if res := startOk.Response; res == "" {
		return nil, fmt.Errorf("no auth response")
	} else {
		p := strings.SplitN(res, "\x00", 3) // null, user, pass
		if len(p) != 3 {
			return nil, fmt.Errorf("invalid auth response %s", res)
		}
		client.user, client.pass = p[1], p[2]
	}

	// Connection.Tune 送信
	tune := &amqp091.ConnectionTune{
		ChannelMax: ChannelMax,
		FrameMax:   FrameMax,
		Heartbeat:  HeartbeatInterval,
	}
	if err := s.send(0, tune); err != nil {
		return nil, fmt.Errorf("failed to write Connection.Tune: %w", err)
	}

	// Connection.Tune-Ok 受信
	tuneOk := amqp091.ConnectionTuneOk{}
	_, err = s.recv(0, &tuneOk)
	if err != nil {
		return nil, fmt.Errorf("failed to read Connection.Tune-Ok: %w", err)
	}

	// Connection.Open 受信
	open := amqp091.ConnectionOpen{}
	_, err = s.recv(0, &open)
	if err != nil {
		return nil, fmt.Errorf("failed to read Connection.Open: %w", err)
	}

	// Connection.Open-Ok 送信
	openOk := &amqp091.ConnectionOpenOk{}
	if err := s.send(0, openOk); err != nil {
		return nil, fmt.Errorf("failed to write Connection.Open-Ok: %w", err)
	}

	if err := s.ConnectToUpstream(config.Upstream.URL, client.user, client.pass); err != nil {
		return nil, fmt.Errorf("failed to connect to upstream: %w", err)
	}
	slog.Info("connected to upstream", "url", config.Upstream.URL)

	return client, nil
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
		slog.Warn("failed to read Connection.Close-Ok", "error", err)
	}
	return nil
}

func (s *Proxy) process(ctx context.Context, client *Client) error {
	frame, err := s.r.ReadFrame()
	if err != nil {
		return fmt.Errorf("failed to read frame: %w", err)
	}
	if mf, ok := frame.(*amqp091.MethodFrame); ok {
		slog.Debug("read method frame", "frame", mf, "type", reflect.TypeOf(mf.Method))
	} else {
		slog.Debug("read frame", "frame", frame, "type", reflect.TypeOf(frame))
	}
	if frame.Channel() == 0 {
		err = s.dispatch0(ctx, client, frame)
	} else {
		err = s.dispatchN(ctx, client, frame)
	}
	if err != nil {
		if e, ok := err.(AMQPError); ok {
			slog.Warn("AMQPError", "error", e)
			return s.send(frame.Channel(), e.AMQPMessage())
		}
		return err
	}
	return nil
}

func (s *Proxy) dispatchN(ctx context.Context, client *Client, frame amqp091.Frame) error {
	switch f := frame.(type) {
	case *amqp091.MethodFrame:
		switch m := f.Method.(type) {
		case *amqp091.ChannelOpen:
			return s.replyChannelOpen(client, f)
		case *amqp091.ChannelClose:
			return s.replyChannelClose(client, f)
		case *amqp091.QueueDeclare:
			return s.replyQueueDeclare(client, f)
		default:
			return NewError(amqp091.NotImplemented, fmt.Sprintf("unsupported method: %T", m))
		}
	case *amqp091.HeartbeatFrame:
		slog.Debug("heartbeat")
		// drop
	default:
		return fmt.Errorf("unsupported frame: %#v", f)
	}
	return nil
}

func (s *Proxy) dispatch0(ctx context.Context, client *Client, frame amqp091.Frame) error {
	switch f := frame.(type) {
	case *amqp091.MethodFrame:
		switch m := f.Method.(type) {
		case *amqp091.ConnectionClose:
			return s.replyConnectionClose(client, f)
		default:
			return fmt.Errorf("unsupported method: %T", m)
		}
	case *amqp091.HeartbeatFrame:
		slog.Debug("heartbeat")
		// drop
	default:
		return fmt.Errorf("unsupported frame: %#v", f)
	}
	//if err := s.dispatch(ctx, client, frame); err != nil {
	//	return fmt.Errorf("failed to dispatch: %w", err)
	//}
	return nil
}

func NewProxy(serverIO io.ReadWriteCloser) *Proxy {
	return &Proxy{
		id: uuid.New().String(),
		r:  amqp091.NewReader(serverIO),
		w:  amqp091.NewWriter(serverIO),
	}
}

func (t *Proxy) send(channel uint16, m amqp091.Message) error {
	slog.Debug("send", "channel", channel, "message", m)
	if msg, ok := m.(amqp091.MessageWithContent); ok {
		props, body := msg.GetContent()
		class, _ := msg.ID()
		if err := t.w.WriteFrame(&amqp091.MethodFrame{
			ChannelId: uint16(channel),
			Method:    msg,
		}); err != nil {
			return fmt.Errorf("WriteFrame error: %w", err)
		}
		if err := t.w.WriteFrame(&amqp091.HeaderFrame{
			ChannelId:  uint16(channel),
			ClassId:    class,
			Size:       uint64(len(body)),
			Properties: props,
		}); err != nil {
			return fmt.Errorf("WriteFrame error: %w", err)
		}
		if err := t.w.WriteFrame(&amqp091.BodyFrame{
			ChannelId: uint16(channel),
			Body:      body,
		}); err != nil {
			return fmt.Errorf("WriteFrame error: %w", err)
		}
	} else {
		if err := t.w.WriteFrame(&amqp091.MethodFrame{
			ChannelId: uint16(channel),
			Method:    m,
		}); err != nil {
			return fmt.Errorf("WriteFrame error: %w", err)
		}
	}
	return nil
}

func (t *Proxy) recv(channel int, m amqp091.Message) (amqp091.Message, error) {
	var remaining int
	var header *amqp091.HeaderFrame
	var body []byte
	defer func() {
		slog.Debug("recv", "channel", channel, "message", m)
	}()

	for {
		frame, err := t.r.ReadFrame()
		if err != nil {
			return nil, fmt.Errorf("frame err, read: %w", err)
		}

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
