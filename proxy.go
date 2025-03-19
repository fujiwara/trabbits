// MIT License
// Copyright (c) 2025 FUJIWARA Shunichiro

package trabbits

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"

	"github.com/fujiwara/trabbits/amqp091"
	rabbitmq "github.com/rabbitmq/amqp091-go"
)

type Proxy struct {
	VirtualHost string

	id     string
	conn   io.ReadWriteCloser
	r      *amqp091.Reader // framer <- client
	w      *amqp091.Writer // framer -> client
	config *Config

	mu sync.Mutex

	upstreams []*Upstream

	logger   *slog.Logger
	user     string
	password string

	keyPatterns []string
}

func NewProxy(conn io.ReadWriteCloser) *Proxy {
	id := generateID()
	return &Proxy{
		conn:   conn,
		id:     id,
		r:      amqp091.NewReader(conn),
		w:      amqp091.NewWriter(conn),
		logger: slog.New(slog.Default().Handler()).With("proxy", id),
	}
}

func (s *Proxy) Upstreams() []*Upstream {
	return s.upstreams
}

func (s *Proxy) GetChannels(id uint16) ([]*rabbitmq.Channel, error) {
	var chs []*rabbitmq.Channel
	for _, us := range s.upstreams {
		ch, err := us.GetChannel(id)
		if err != nil {
			return nil, err
		}
		chs = append(chs, ch)
	}
	return chs, nil
}

func (s *Proxy) GetChannel(id uint16, routingKey string) (*rabbitmq.Channel, error) {
	var routed *Upstream
	for _, us := range s.upstreams {
		for _, pattern := range us.keyPatterns {
			if matchPattern(routingKey, pattern) {
				routed = us
				us.logger.Debug("matched pattern", "pattern", pattern, "routing_key", routingKey)
				break
			}
		}
	}
	if routed != nil {
		return routed.GetChannel(id)
	}
	us := s.upstreams[0] // default upstream
	us.logger.Debug("not matched any patterns, using default upstream", "routing_key", routingKey)
	return us.GetChannel(id)
}

func (s *Proxy) ClientAddr() string {
	if s.conn == nil {
		return ""
	}
	if tcpConn, ok := s.conn.(*net.TCPConn); ok {
		return tcpConn.RemoteAddr().String()
	}
	return ""
}

func (s *Proxy) Close() {
	for _, us := range s.Upstreams() {
		if err := us.Close(); err != nil {
			us.logger.Warn("failed to close upstream", "error", err)
		}
	}
}

func (s *Proxy) NewChannel(id uint16) error {
	for _, us := range s.Upstreams() {
		if _, err := us.NewChannel(id); err != nil {
			return fmt.Errorf("failed to create channel: %w", err)
		}
	}
	return nil
}

func (s *Proxy) CloseChannel(id uint16) error {
	for _, us := range s.Upstreams() {
		if err := us.CloseChannel(id); err != nil {
			return err
		}
	}
	return nil
}
