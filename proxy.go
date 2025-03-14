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

	defaultUpstream *Upstream
	anotherUpstream *Upstream

	logger   *slog.Logger
	user     string
	password string
}

type Upstream struct {
	conn    *rabbitmq.Connection
	channel map[uint16]*rabbitmq.Channel
	mu      sync.Mutex
}

func NewUpstream(conn *rabbitmq.Connection) *Upstream {
	return &Upstream{
		conn:    conn,
		channel: make(map[uint16]*rabbitmq.Channel),
		mu:      sync.Mutex{},
	}
}

func (u *Upstream) Close() error {
	if u == nil {
		return nil
	}
	if u.conn != nil {
		if err := u.conn.Close(); err != nil {
			return fmt.Errorf("failed to close connection: %w", err)
		}
	}
	// all channels are closed by connection.Close(), so no need to close them here
	return nil
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
	us := make([]*Upstream, 0, 2)
	if s.defaultUpstream != nil {
		us = append(us, s.defaultUpstream)
	}
	if s.anotherUpstream != nil {
		us = append(us, s.anotherUpstream)
	}
	return us
}

func (s *Proxy) GetChannels(id uint16) ([]*rabbitmq.Channel, error) {
	var chs []*rabbitmq.Channel
	for _, us := range s.Upstreams() {
		ch, err := us.GetChannel(id)
		if err != nil {
			return nil, err
		}
		chs = append(chs, ch)
	}
	return chs, nil
}

func (s *Proxy) GetChannel(id uint16, routingKey string) (*rabbitmq.Channel, error) {
	// TODO implement routing
	us := s.defaultUpstream
	if us == nil {
		return nil, fmt.Errorf("default upstream not found")
	}
	if ch, ok := us.channel[id]; !ok {
		return nil, fmt.Errorf("channel %d not found", id)
	} else {
		return ch, nil
	}
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
			s.logger.Warn("failed to close upstream", "error", err)
		}
	}
}

func (u *Upstream) NewChannel(id uint16) (*rabbitmq.Channel, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if _, ok := u.channel[id]; ok {
		return nil, fmt.Errorf("channel %d already exists", id)
	}
	ch, err := u.conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("failed to create upstream channel: %w", err)
	}
	u.channel[id] = ch
	return u.channel[id], nil
}

func (u *Upstream) GetChannel(id uint16) (*rabbitmq.Channel, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if ch, ok := u.channel[id]; ok {
		return ch, nil
	}
	return nil, fmt.Errorf("channel %d not found", id)
}

func (u *Upstream) CloseChannel(id uint16) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if ch, ok := u.channel[id]; !ok {
		return fmt.Errorf("channel %d not found", id)
	} else {
		if err := ch.Close(); err != nil {
			return fmt.Errorf("failed to close channel %d: %w", id, err)
		}
	}
	delete(u.channel, id)
	return nil
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
