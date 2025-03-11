package trabbits

import (
	"fmt"
	"log/slog"

	"github.com/fujiwara/trabbits/amqp091"
)

func (s *Proxy) replyChannelOpen(client *Client, f *amqp091.MethodFrame) error {
	id := f.Channel()
	slog.Debug("Channel.Open", "channel", id, "client", client.id)

	ch, err := s.upstream.Channel()
	if err != nil {
		return fmt.Errorf("failed to create upstream channel: %w", err)
	}
	if _, err := client.NewChannel(id, ch); err != nil {
		return fmt.Errorf("failed to create channel: %w", err)
	}

	if err := s.send(id, &amqp091.ChannelOpenOk{}); err != nil {
		return fmt.Errorf("failed to write Channel.Open-Ok: %w", err)
	}
	return nil
}

func (s *Proxy) replyChannelClose(client *Client, f *amqp091.MethodFrame) error {
	id := f.Channel()
	slog.Debug("Channel.Close", "channel", id, "client", client.id)
	if err := client.CloseChannel(id); err != nil {
		return err
	}
	return s.send(id, &amqp091.ChannelCloseOk{})
}

func (s *Proxy) replyConnectionClose(_ *Client, _ *amqp091.MethodFrame) error {
	return s.send(0, &amqp091.ConnectionCloseOk{})
}

func (s *Proxy) replyQueueDeclare(client *Client, f *amqp091.MethodFrame) error {
	id := f.Channel()
	ch, err := client.GetChannel(id)
	if err != nil {
		return err
	}
	m, ok := f.Method.(*amqp091.QueueDeclare)
	if !ok {
		panic("invalid method")
	}
	q, err := ch.QueueDeclare(
		m.Queue,
		m.Durable,
		m.AutoDelete,
		m.Exclusive,
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		return fmt.Errorf("failed to declare queue on upstream: %w", err)
	}
	slog.Debug("Queue.Declare", "queue", q, "client", client.id)
	return s.send(id, &amqp091.QueueDeclareOk{
		Queue:         q.Name,
		MessageCount:  uint32(q.Messages),
		ConsumerCount: uint32(q.Consumers),
	})
}
