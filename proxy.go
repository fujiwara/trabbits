// MIT License
// Copyright (c) 2025 FUJIWARA Shunichiro

package trabbits

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/fujiwara/trabbits/amqp091"
	"github.com/fujiwara/trabbits/pattern"
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

	configHash         string      // hash of config used for this proxy
	upstreamDisconnect chan string // channel to notify upstream disconnection
}

func NewProxy(conn net.Conn) *Proxy {
	id := generateID()
	p := &Proxy{
		conn:               conn,
		id:                 id,
		r:                  amqp091.NewReader(conn),
		w:                  amqp091.NewWriter(conn),
		upstreamDisconnect: make(chan string, 10), // buffered to avoid blocking
	}
	p.logger = slog.New(slog.Default().Handler()).With("proxy", id, "client_addr", p.ClientAddr())
	return p
}

func (p *Proxy) Upstreams() []*Upstream {
	return p.upstreams
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
				us.logger.Debug("matched pattern", "pattern", keyPattern, "routing_key", routingKey)
				break
			}
		}
	}
	if routed != nil {
		return routed.GetChannel(id)
	}
	us := p.upstreams[0] // default upstream
	us.logger.Debug("not matched any patterns, using default upstream", "routing_key", routingKey)
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
	select {
	case <-ctx.Done():
		p.logger.Debug("Upstream monitoring stopped by context", "upstream", upstream.String())
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
