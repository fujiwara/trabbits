package trabbits

import (
	"fmt"
	"log/slog"

	"github.com/fujiwara/trabbits/amqp091"
)

func (s *Server) replyChannelOpen(client *Client, id uint16) error {
	slog.Info("Channel.Open", "channel", id, "client", client.id)
	_, err := client.NewChannel(id)
	if err != nil {
		return fmt.Errorf("failed to create channel: %w", err)
	}
	if err := s.send(id, &amqp091.ChannelOpenOk{}); err != nil {
		return fmt.Errorf("failed to write Channel.Open-Ok: %w", err)
	}
	return nil
}

func (s *Server) replyChannelClose(client *Client, id uint16) error {
	slog.Info("Channel.Close", "channel", id, "client", client.id)
	if err := client.CloseChannel(id); err != nil {
		return err
	}
	slog.Info("Channel.Close", "channel", id)
	return s.send(id, &amqp091.ChannelCloseOk{})
}

func (s *Server) replyConnectionClose(msg *amqp091.ConnectionClose) error {
	return s.send(0, &amqp091.ConnectionCloseOk{})
}
