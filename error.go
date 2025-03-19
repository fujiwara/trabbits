// MIT License
// Copyright (c) 2025 FUJIWARA Shunichiro

package trabbits

import (
	"fmt"

	"github.com/fujiwara/trabbits/amqp091"
)

type AMQPError interface {
	AMQPMessage() amqp091.Message
	Error() string
}

type Error struct {
	Code    uint16
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Message)
}

func NewError(code uint16, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

type ChannelError struct {
	Error
}

func NewChannelError(code uint16, message string) *ChannelError {
	return &ChannelError{
		Error: Error{
			Code:    code,
			Message: message,
		},
	}
}

func (e *ChannelError) AMQPMessage() amqp091.Message {
	return &amqp091.ChannelClose{
		ReplyCode: e.Code,
		ReplyText: e.Message,
	}
}
