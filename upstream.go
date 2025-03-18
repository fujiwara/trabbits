package trabbits

import (
	"fmt"
	"log/slog"
	"sync"

	rabbitmq "github.com/rabbitmq/amqp091-go"
)

type Upstream struct {
	conn        *rabbitmq.Connection
	channel     map[uint16]*rabbitmq.Channel
	mu          sync.Mutex
	logger      *slog.Logger
	keyPatterns []string
}

func NewUpstream(conn *rabbitmq.Connection, logger *slog.Logger, conf UpstreamConfig) *Upstream {
	return &Upstream{
		conn:        conn,
		channel:     make(map[uint16]*rabbitmq.Channel),
		mu:          sync.Mutex{},
		logger:      logger.With("upstream", conn.RemoteAddr().String()),
		keyPatterns: conf.Routing.KeyPatterns,
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
	u.logger.Debug("channel created", "id", id)
	return u.channel[id], nil
}

func (u *Upstream) GetChannel(id uint16) (*rabbitmq.Channel, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if ch, ok := u.channel[id]; ok {
		u.logger.Debug("got channel", "id", id)
		return ch, nil
	}
	return nil, fmt.Errorf("channel %d not found", id)
}

func (u *Upstream) CloseChannel(id uint16) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.logger.Debug("closing channel", "id", id)
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
