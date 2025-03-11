package trabbits

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/fujiwara/trabbits/amqp091"
	rabbitmq "github.com/rabbitmq/amqp091-go"
)

func (s *Proxy) replyChannelOpen(ctx context.Context, client *Client, f *amqp091.MethodFrame, _ *amqp091.ChannelOpen) error {
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

func (s *Proxy) replyChannelClose(_ context.Context, client *Client, f *amqp091.MethodFrame, _ *amqp091.ChannelClose) error {
	id := f.Channel()
	slog.Debug("Channel.Close", "channel", id, "client", client.id)
	if err := client.CloseChannel(id); err != nil {
		return err
	}
	return s.send(id, &amqp091.ChannelCloseOk{})
}

func (s *Proxy) replyConnectionClose(_ context.Context, _ *Client, _ *amqp091.MethodFrame, _ *amqp091.ConnectionClose) error {
	return s.send(0, &amqp091.ConnectionCloseOk{})
}

func (s *Proxy) replyQueueDeclare(_ context.Context, client *Client, f *amqp091.MethodFrame, m *amqp091.QueueDeclare) error {
	id := f.Channel()
	ch, err := client.GetChannel(id)
	if err != nil {
		return err
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

func (s *Proxy) replyBasicPublish(ctx context.Context, client *Client, f *amqp091.MethodFrame, m *amqp091.BasicPublish) error {
	id := f.Channel()
	ch, err := client.GetChannel(id)
	if err != nil {
		return err
	}
	slog.Debug("Basic.Publish",
		"exchange", m.Exchange, "routing_key", m.RoutingKey, "client", client.id,
		"body", string(m.Body), "properties", m.Properties,
	)
	if err := ch.PublishWithContext(
		ctx,
		m.Exchange,
		m.RoutingKey,
		m.Mandatory,
		m.Immediate,
		rabbitmq.Publishing{
			Body:            m.Body,
			AppId:           m.Properties.AppId,
			ContentEncoding: m.Properties.ContentEncoding,
			ContentType:     m.Properties.ContentType,
			CorrelationId:   m.Properties.CorrelationId,
			DeliveryMode:    m.Properties.DeliveryMode,
			Expiration:      m.Properties.Expiration,
			MessageId:       m.Properties.MessageId,
			ReplyTo:         m.Properties.ReplyTo,
			Timestamp:       m.Properties.Timestamp,
			Type:            m.Properties.Type,
			UserId:          m.Properties.UserId,
			Headers:         rabbitmq.Table(m.Properties.Headers),
		},
	); err != nil {
		return NewError(amqp091.InternalError, fmt.Sprintf("failed to publish message: %v", err))
	}
	return nil
}
