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

	id   string
	conn io.ReadWriteCloser
	r    *amqp091.Reader // framer <- client
	w    *amqp091.Writer // framer -> client

	mu       sync.Mutex
	upstream *rabbitmq.Connection
	channel  map[uint16]*rabbitmq.Channel
	user     string
	password string
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
	if s.upstream != nil {
		if err := s.upstream.Close(); err != nil {
			slog.Warn("failed to close upstream", "error", err)
		}
	}
	if s.conn != nil {
		s.conn.Close()
	}
}

func (s *Proxy) NewChannel(id uint16, ch *rabbitmq.Channel) (*rabbitmq.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.channel[id]; ok {
		return nil, fmt.Errorf("channel %d already exists", id)
	}
	s.channel[id] = ch
	return s.channel[id], nil
}

func (s *Proxy) GetChannel(id uint16) (*rabbitmq.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ch, ok := s.channel[id]; ok {
		return ch, nil
	}
	return nil, fmt.Errorf("channel %d not found", id)
}

func (s *Proxy) CloseChannel(id uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ch, ok := s.channel[id]; !ok {
		return fmt.Errorf("channel %d not found", id)
	} else {
		ch.Close()
	}
	delete(s.channel, id)
	return nil
}
