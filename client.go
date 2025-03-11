package trabbits

import (
	"fmt"
	"net"
	"sync"

	"github.com/google/uuid"
	rabbitmq "github.com/rabbitmq/amqp091-go"
)

type Client struct {
	id      string
	conn    net.Conn
	channel map[uint16]*rabbitmq.Channel
	user    string
	pass    string
	mu      *sync.Mutex
}

func NewClient(conn net.Conn) *Client {
	return &Client{
		id:      uuid.New().String(),
		conn:    conn,
		channel: make(map[uint16]*rabbitmq.Channel),
		mu:      &sync.Mutex{},
	}
}

func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Client) NewChannel(id uint16, ch *rabbitmq.Channel) (*rabbitmq.Channel, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.channel[id]; ok {
		return nil, fmt.Errorf("channel %d already exists", id)
	}
	c.channel[id] = ch
	return c.channel[id], nil
}

func (c *Client) GetChannel(id uint16) (*rabbitmq.Channel, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ch, ok := c.channel[id]; ok {
		return ch, nil
	}
	return nil, fmt.Errorf("channel %d not found", id)
}

func (c *Client) CloseChannel(id uint16) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ch, ok := c.channel[id]; !ok {
		return fmt.Errorf("channel %d not found", id)
	} else {
		ch.Close()
	}
	delete(c.channel, id)
	return nil
}
