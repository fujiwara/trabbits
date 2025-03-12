package trabbits

import (
	"context"
	"fmt"

	"github.com/fujiwara/trabbits/amqp091"
	rabbitmq "github.com/rabbitmq/amqp091-go"
)

func (s *Proxy) replyChannelOpen(ctx context.Context, f *amqp091.MethodFrame, _ *amqp091.ChannelOpen) error {
	id := f.Channel()
	s.logger.Debug("Channel.Open", "channel", id, "proxy", s.id)

	ch, err := s.upstream.Channel()
	if err != nil {
		return fmt.Errorf("failed to create upstream channel: %w", err)
	}
	if _, err := s.NewChannel(id, ch); err != nil {
		return fmt.Errorf("failed to create channel: %w", err)
	}

	if err := s.send(id, &amqp091.ChannelOpenOk{}); err != nil {
		return fmt.Errorf("failed to write Channel.Open-Ok: %w", err)
	}
	return nil
}

func (s *Proxy) replyChannelClose(_ context.Context, f *amqp091.MethodFrame, _ *amqp091.ChannelClose) error {
	id := f.Channel()
	s.logger.Debug("Channel.Close", "channel", id)
	if err := s.CloseChannel(id); err != nil {
		return err
	}
	return s.send(id, &amqp091.ChannelCloseOk{})
}

func (s *Proxy) replyConnectionClose(_ context.Context, _ *amqp091.MethodFrame, _ *amqp091.ConnectionClose) error {
	return s.send(0, &amqp091.ConnectionCloseOk{})
}

func (s *Proxy) replyQueueDeclare(_ context.Context, f *amqp091.MethodFrame, m *amqp091.QueueDeclare) error {
	id := f.Channel()
	ch, err := s.GetChannel(id)
	if err != nil {
		return err
	}
	q, err := ch.QueueDeclare(
		m.Queue,
		m.Durable,
		m.AutoDelete,
		m.Exclusive,
		false, // no-wait
		rabbitmq.Table(m.Arguments),
	)
	if err != nil {
		return fmt.Errorf("failed to declare queue on upstream: %w", err)
	}
	s.logger.Debug("Queue.Declare", "queue", q)
	return s.send(id, &amqp091.QueueDeclareOk{
		Queue:         q.Name,
		MessageCount:  uint32(q.Messages),
		ConsumerCount: uint32(q.Consumers),
	})
}

func (s *Proxy) replyBasicPublish(ctx context.Context, f *amqp091.MethodFrame, m *amqp091.BasicPublish) error {
	id := f.Channel()
	ch, err := s.GetChannel(id)
	if err != nil {
		return err
	}
	s.logger.Debug("Basic.Publish",
		"exchange", m.Exchange, "routing_key", m.RoutingKey,
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

func (s *Proxy) replyBasicConsume(ctx context.Context, f *amqp091.MethodFrame, m *amqp091.BasicConsume) error {
	id := f.Channel()
	ch, err := s.GetChannel(id)
	if err != nil {
		return err
	}
	s.logger.Debug("Basic.Consume", "queue", m.Queue)
	consume, err := ch.ConsumeWithContext(
		ctx,
		m.Queue,
		m.ConsumerTag,
		m.NoAck,
		m.Exclusive,
		m.NoLocal,
		m.NoWait,
		rabbitmq.Table(m.Arguments),
	)
	if err != nil {
		return NewError(amqp091.InternalError, fmt.Sprintf("failed to consume: %v", err))
	}
	if err := s.send(id, &amqp091.BasicConsumeOk{
		ConsumerTag: m.ConsumerTag,
	}); err != nil {
		return NewError(amqp091.InternalError, fmt.Sprintf("failed to send Basic.ConsumeOk: %v", err))
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-consume:
				if !ok {
					s.logger.Debug("Basic.Consume closed", "queue", m.Queue)
				}
				s.logger.Debug("Basic.Deliver", "msg", msg)
				err := s.send(id, &amqp091.BasicDeliver{
					ConsumerTag: m.ConsumerTag,
					DeliveryTag: msg.DeliveryTag,
					Redelivered: msg.Redelivered,
					Exchange:    msg.Exchange,
					RoutingKey:  msg.RoutingKey,
					Body:        msg.Body,
					Properties:  deliveryToProps(&msg),
				})
				if err != nil {
					s.logger.Warn("failed to deliver message", "error", err)
				}
			}
		}
	}()
	return nil
}

func (s *Proxy) replyBasicGet(ctx context.Context, f *amqp091.MethodFrame, m *amqp091.BasicGet) error {
	id := f.Channel()
	ch, err := s.GetChannel(id)
	if err != nil {
		return err
	}
	s.logger.Debug("Basic.Get", "queue", m.Queue)
	msg, ok, err := ch.Get(m.Queue, m.NoAck)
	if err != nil {
		return NewError(amqp091.InternalError, fmt.Sprintf("failed to get message: %v", err))
	}
	if !ok {
		return s.send(id, &amqp091.BasicGetEmpty{})
	}
	s.logger.Debug("Basic.Get", "msg", msg)
	return s.send(id, &amqp091.BasicGetOk{
		DeliveryTag:  msg.DeliveryTag,
		Redelivered:  msg.Redelivered,
		Exchange:     msg.Exchange,
		RoutingKey:   msg.RoutingKey,
		MessageCount: uint32(msg.MessageCount),
		Body:         msg.Body,
		Properties:   deliveryToProps(&msg),
	})
}

func (s *Proxy) replyBasicAck(_ context.Context, f *amqp091.MethodFrame, m *amqp091.BasicAck) error {
	id := f.Channel()
	ch, err := s.GetChannel(id)
	if err != nil {
		return err
	}
	s.logger.Debug("Basic.Ack", "delivery_tag", m.DeliveryTag, "multiple", m.Multiple)
	return ch.Ack(m.DeliveryTag, m.Multiple)
}

func (s *Proxy) replyBasicNack(_ context.Context, f *amqp091.MethodFrame, m *amqp091.BasicNack) error {
	id := f.Channel()
	ch, err := s.GetChannel(id)
	if err != nil {
		return err
	}
	s.logger.Debug("Basic.Nack", "delivery_tag", m.DeliveryTag, "multiple", m.Multiple, "requeue", m.Requeue)
	return ch.Nack(m.DeliveryTag, m.Multiple, m.Requeue)
}

func deliveryToProps(msg *rabbitmq.Delivery) amqp091.Properties {
	return amqp091.Properties{
		ContentType:     msg.ContentType,
		ContentEncoding: msg.ContentEncoding,
		DeliveryMode:    msg.DeliveryMode,
		Priority:        msg.Priority,
		CorrelationId:   msg.CorrelationId,
		ReplyTo:         msg.ReplyTo,
		Expiration:      msg.Expiration,
		MessageId:       msg.MessageId,
		Timestamp:       msg.Timestamp,
		Type:            msg.Type,
		UserId:          msg.UserId,
		AppId:           msg.AppId,
		Headers:         amqp091.Table(msg.Headers),
	}
}

func (s *Proxy) replyBasicCancel(_ context.Context, f *amqp091.MethodFrame, m *amqp091.BasicCancel) error {
	id := f.Channel()
	ch, err := s.GetChannel(id)
	if err != nil {
		return err
	}
	s.logger.Debug("Basic.Cancel", "consumer_tag", m.ConsumerTag)
	return ch.Cancel(m.ConsumerTag, false)
}

func (s *Proxy) replyQueueDelete(_ context.Context, f *amqp091.MethodFrame, m *amqp091.QueueDelete) error {
	id := f.Channel()
	ch, err := s.GetChannel(id)
	if err != nil {
		return err
	}
	s.logger.Debug("Queue.Delete", "queue", m.Queue)
	if _, err := ch.QueueDelete(
		m.Queue,
		m.IfUnused,
		m.IfEmpty,
		m.NoWait,
	); err != nil {
		return NewError(amqp091.InternalError, fmt.Sprintf("failed to delete queue: %v", err))
	}
	return s.send(id, &amqp091.QueueDeleteOk{})
}
