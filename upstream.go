// MIT License
// Copyright (c) 2025 FUJIWARA Shunichiro

package trabbits

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/fujiwara/trabbits/amqp091"
	rabbitmq "github.com/rabbitmq/amqp091-go"
)

type Upstream struct {
	conn        *rabbitmq.Connection
	channel     map[uint16]*rabbitmq.Channel
	mu          sync.Mutex
	logger      *slog.Logger
	keyPatterns []string
	addr        string
	queueAttr   *QueueAttributes
}

func NewUpstream(addr string, conn *rabbitmq.Connection, logger *slog.Logger, conf UpstreamConfig) *Upstream {
	u := &Upstream{
		conn:        conn,
		channel:     make(map[uint16]*rabbitmq.Channel),
		mu:          sync.Mutex{},
		logger:      logger.With("upstream", addr),
		keyPatterns: conf.Routing.KeyPatterns,
		addr:        addr,
		queueAttr:   conf.QueueAttributes,
	}
	metrics.UpstreamTotalConnections.WithLabelValues(addr).Inc()
	metrics.UpstreamConnections.WithLabelValues(addr).Inc()
	return u
}

func (u *Upstream) String() string {
	return u.addr
}

func (u *Upstream) Close() error {
	if u == nil {
		return nil
	}
	if u.conn != nil {
		metrics.UpstreamConnections.WithLabelValues(u.String()).Dec()
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

const AutoGenerateQueueNamePrefix = "trabbits.gen-"

// QueueDeclareArgs generates arguments for rabbitmq.QueueDeclare(name, durable, autoDelete, exclusive, noWait bool, args Table)
// noWait is always false because we need to wait for the response from multiple upstreams
func (u *Upstream) QueueDeclareArgs(m *amqp091.QueueDeclare) (name string, durable bool, autoDelete bool, exclusive bool, noWait bool, args rabbitmq.Table) {
	if strings.HasPrefix(m.Queue, AutoGenerateQueueNamePrefix) {
		// auto-generate queue name like amq.gen-xxxxx
		// durable: false, autoDelete: true, exclusive: true because it's the same as amq.gen-xxxxx
		return m.Queue, false, true, true, false, rabbitmq.Table(m.Arguments)
	}
	if attr := u.queueAttr; attr == nil {
		// if no queue attributes, respect the message's attributes (excluding noWait)
		return m.Queue, m.Durable, m.AutoDelete, m.Exclusive, false, rabbitmq.Table(m.Arguments)
	} else {
		u.logger.Debug("overriding queue attributes", "queue", m.Queue, "upstream", u.String(), "attributes", *attr)
		// override the message's attributes with the upstream's configuration
		durable := m.Durable
		if attr.Durable != nil {
			durable = *attr.Durable
		}
		autoDelete := m.AutoDelete
		if attr.AutoDelete != nil {
			autoDelete = *attr.AutoDelete
		}
		exclusive := m.Exclusive
		if attr.Exclusive != nil {
			exclusive = *attr.Exclusive
		}
		arguments := rabbitmq.Table(m.Arguments)
		if attr.Arguments != nil {
			for k, v := range attr.Arguments {
				if v == nil {
					delete(arguments, k)
				} else {
					arguments[k] = v
				}
			}
		}
		return m.Queue, durable, autoDelete, exclusive, false, arguments
	}
}
