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
	Code() uint16
}

type Error struct {
	code    uint16
	message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%d: %s", e.code, e.message)
}

func (e *Error) Code() uint16 {
	return e.code
}

func (e *Error) AMQPMessage() amqp091.Message {
	return &amqp091.ConnectionClose{
		ReplyCode: e.code,
		ReplyText: e.message,
	}
}

func NewError(code uint16, message string) *Error {
	return &Error{
		code:    code,
		message: message,
	}
}

type ChannelError struct {
	Error
}

func NewChannelError(code uint16, message string) *ChannelError {
	return &ChannelError{
		Error: Error{
			code:    code,
			message: message,
		},
	}
}

func (e *ChannelError) AMQPMessage() amqp091.Message {
	return &amqp091.ChannelClose{
		ReplyCode: e.code,
		ReplyText: e.message,
	}
}
