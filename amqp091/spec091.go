// Copyright (c) 2021 VMware, Inc. or its affiliates. All Rights Reserved.
// Copyright (c) 2012-2021, Sean Treadway, SoundCloud Ltd.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/* GENERATED FILE - DO NOT EDIT */
/* Rebuild from the spec/gen.go tool */

package amqp091

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Error codes that can be sent from the server during a connection or
// channel exception or used by the client to indicate a class of error like
// ErrCredentials.  The text of the error is likely more interesting than
// these constants.
const (
	frameMethod        = 1
	frameHeader        = 2
	frameBody          = 3
	frameHeartbeat     = 8
	frameMinSize       = 4096
	frameEnd           = 206
	replySuccess       = 200
	ContentTooLarge    = 311
	NoRoute            = 312
	NoConsumers        = 313
	ConnectionForced   = 320
	InvalidPath        = 402
	AccessRefused      = 403
	NotFound           = 404
	ResourceLocked     = 405
	PreconditionFailed = 406
	FrameError         = 501
	SyntaxError        = 502
	CommandInvalid     = 503
	ChannelError       = 504
	UnexpectedFrame    = 505
	ResourceError      = 506
	NotAllowed         = 530
	NotImplemented     = 540
	InternalError      = 541
)

func isSoftExceptionCode(code int) bool {
	switch code {
	case 311:
		return true
	case 312:
		return true
	case 313:
		return true
	case 403:
		return true
	case 404:
		return true
	case 405:
		return true
	case 406:
		return true

	}
	return false
}

type ConnectionStart struct {
	VersionMajor     byte
	VersionMinor     byte
	ServerProperties Table
	Mechanisms       string
	Locales          string
}

func (msg *ConnectionStart) ID() (uint16, uint16) {
	return 10, 10
}

func (msg *ConnectionStart) Wait() bool {
	return true
}

func (msg *ConnectionStart) Write(w io.Writer) (err error) {

	if err = binary.Write(w, binary.BigEndian, msg.VersionMajor); err != nil {
		return
	}
	if err = binary.Write(w, binary.BigEndian, msg.VersionMinor); err != nil {
		return
	}

	if err = writeTable(w, msg.ServerProperties); err != nil {
		return
	}

	if err = writeLongstr(w, msg.Mechanisms); err != nil {
		return
	}
	if err = writeLongstr(w, msg.Locales); err != nil {
		return
	}

	return
}

func (msg *ConnectionStart) Read(r io.Reader) (err error) {

	if err = binary.Read(r, binary.BigEndian, &msg.VersionMajor); err != nil {
		return
	}
	if err = binary.Read(r, binary.BigEndian, &msg.VersionMinor); err != nil {
		return
	}

	if msg.ServerProperties, err = readTable(r); err != nil {
		return
	}

	if msg.Mechanisms, err = readLongstr(r); err != nil {
		return
	}
	if msg.Locales, err = readLongstr(r); err != nil {
		return
	}

	return
}

type ConnectionStartOk struct {
	ClientProperties Table
	Mechanism        string
	Response         string
	Locale           string
}

func (msg *ConnectionStartOk) ID() (uint16, uint16) {
	return 10, 11
}

func (msg *ConnectionStartOk) Wait() bool {
	return true
}

func (msg *ConnectionStartOk) Write(w io.Writer) (err error) {

	if err = writeTable(w, msg.ClientProperties); err != nil {
		return
	}

	if err = writeShortstr(w, msg.Mechanism); err != nil {
		return
	}

	if err = writeLongstr(w, msg.Response); err != nil {
		return
	}

	if err = writeShortstr(w, msg.Locale); err != nil {
		return
	}

	return
}

func (msg *ConnectionStartOk) Read(r io.Reader) (err error) {

	if msg.ClientProperties, err = readTable(r); err != nil {
		return
	}

	if msg.Mechanism, err = readShortstr(r); err != nil {
		return
	}

	if msg.Response, err = readLongstr(r); err != nil {
		return
	}

	if msg.Locale, err = readShortstr(r); err != nil {
		return
	}

	return
}

type ConnectionSecure struct {
	Challenge string
}

func (msg *ConnectionSecure) ID() (uint16, uint16) {
	return 10, 20
}

func (msg *ConnectionSecure) Wait() bool {
	return true
}

func (msg *ConnectionSecure) Write(w io.Writer) (err error) {

	if err = writeLongstr(w, msg.Challenge); err != nil {
		return
	}

	return
}

func (msg *ConnectionSecure) Read(r io.Reader) (err error) {

	if msg.Challenge, err = readLongstr(r); err != nil {
		return
	}

	return
}

type ConnectionSecureOk struct {
	Response string
}

func (msg *ConnectionSecureOk) ID() (uint16, uint16) {
	return 10, 21
}

func (msg *ConnectionSecureOk) Wait() bool {
	return true
}

func (msg *ConnectionSecureOk) Write(w io.Writer) (err error) {

	if err = writeLongstr(w, msg.Response); err != nil {
		return
	}

	return
}

func (msg *ConnectionSecureOk) Read(r io.Reader) (err error) {

	if msg.Response, err = readLongstr(r); err != nil {
		return
	}

	return
}

type ConnectionTune struct {
	ChannelMax uint16
	FrameMax   uint32
	Heartbeat  uint16
}

func (msg *ConnectionTune) ID() (uint16, uint16) {
	return 10, 30
}

func (msg *ConnectionTune) Wait() bool {
	return true
}

func (msg *ConnectionTune) Write(w io.Writer) (err error) {

	if err = binary.Write(w, binary.BigEndian, msg.ChannelMax); err != nil {
		return
	}

	if err = binary.Write(w, binary.BigEndian, msg.FrameMax); err != nil {
		return
	}

	if err = binary.Write(w, binary.BigEndian, msg.Heartbeat); err != nil {
		return
	}

	return
}

func (msg *ConnectionTune) Read(r io.Reader) (err error) {

	if err = binary.Read(r, binary.BigEndian, &msg.ChannelMax); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &msg.FrameMax); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &msg.Heartbeat); err != nil {
		return
	}

	return
}

type ConnectionTuneOk struct {
	ChannelMax uint16
	FrameMax   uint32
	Heartbeat  uint16
}

func (msg *ConnectionTuneOk) ID() (uint16, uint16) {
	return 10, 31
}

func (msg *ConnectionTuneOk) Wait() bool {
	return true
}

func (msg *ConnectionTuneOk) Write(w io.Writer) (err error) {

	if err = binary.Write(w, binary.BigEndian, msg.ChannelMax); err != nil {
		return
	}

	if err = binary.Write(w, binary.BigEndian, msg.FrameMax); err != nil {
		return
	}

	if err = binary.Write(w, binary.BigEndian, msg.Heartbeat); err != nil {
		return
	}

	return
}

func (msg *ConnectionTuneOk) Read(r io.Reader) (err error) {

	if err = binary.Read(r, binary.BigEndian, &msg.ChannelMax); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &msg.FrameMax); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &msg.Heartbeat); err != nil {
		return
	}

	return
}

type ConnectionOpen struct {
	VirtualHost string
	reserved1   string
	reserved2   bool
}

func (msg *ConnectionOpen) ID() (uint16, uint16) {
	return 10, 40
}

func (msg *ConnectionOpen) Wait() bool {
	return true
}

func (msg *ConnectionOpen) Write(w io.Writer) (err error) {
	var bits byte

	if err = writeShortstr(w, msg.VirtualHost); err != nil {
		return
	}
	if err = writeShortstr(w, msg.reserved1); err != nil {
		return
	}

	if msg.reserved2 {
		bits |= 1 << 0
	}

	if err = binary.Write(w, binary.BigEndian, bits); err != nil {
		return
	}

	return
}

func (msg *ConnectionOpen) Read(r io.Reader) (err error) {
	var bits byte

	if msg.VirtualHost, err = readShortstr(r); err != nil {
		return
	}
	if msg.reserved1, err = readShortstr(r); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &bits); err != nil {
		return
	}
	msg.reserved2 = (bits&(1<<0) > 0)

	return
}

type ConnectionOpenOk struct {
	reserved1 string
}

func (msg *ConnectionOpenOk) ID() (uint16, uint16) {
	return 10, 41
}

func (msg *ConnectionOpenOk) Wait() bool {
	return true
}

func (msg *ConnectionOpenOk) Write(w io.Writer) (err error) {

	if err = writeShortstr(w, msg.reserved1); err != nil {
		return
	}

	return
}

func (msg *ConnectionOpenOk) Read(r io.Reader) (err error) {

	if msg.reserved1, err = readShortstr(r); err != nil {
		return
	}

	return
}

type ConnectionClose struct {
	ReplyCode uint16
	ReplyText string
	ClassId   uint16
	MethodId  uint16
}

func (msg *ConnectionClose) ID() (uint16, uint16) {
	return 10, 50
}

func (msg *ConnectionClose) Wait() bool {
	return true
}

func (msg *ConnectionClose) Write(w io.Writer) (err error) {

	if err = binary.Write(w, binary.BigEndian, msg.ReplyCode); err != nil {
		return
	}

	if err = writeShortstr(w, msg.ReplyText); err != nil {
		return
	}

	if err = binary.Write(w, binary.BigEndian, msg.ClassId); err != nil {
		return
	}
	if err = binary.Write(w, binary.BigEndian, msg.MethodId); err != nil {
		return
	}

	return
}

func (msg *ConnectionClose) Read(r io.Reader) (err error) {

	if err = binary.Read(r, binary.BigEndian, &msg.ReplyCode); err != nil {
		return
	}

	if msg.ReplyText, err = readShortstr(r); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &msg.ClassId); err != nil {
		return
	}
	if err = binary.Read(r, binary.BigEndian, &msg.MethodId); err != nil {
		return
	}

	return
}

type ConnectionCloseOk struct {
}

func (msg *ConnectionCloseOk) ID() (uint16, uint16) {
	return 10, 51
}

func (msg *ConnectionCloseOk) Wait() bool {
	return true
}

func (msg *ConnectionCloseOk) Write(w io.Writer) (err error) {

	return
}

func (msg *ConnectionCloseOk) Read(r io.Reader) (err error) {

	return
}

type ConnectionBlocked struct {
	Reason string
}

func (msg *ConnectionBlocked) ID() (uint16, uint16) {
	return 10, 60
}

func (msg *ConnectionBlocked) Wait() bool {
	return false
}

func (msg *ConnectionBlocked) Write(w io.Writer) (err error) {

	if err = writeShortstr(w, msg.Reason); err != nil {
		return
	}

	return
}

func (msg *ConnectionBlocked) Read(r io.Reader) (err error) {

	if msg.Reason, err = readShortstr(r); err != nil {
		return
	}

	return
}

type ConnectionUnblocked struct {
}

func (msg *ConnectionUnblocked) ID() (uint16, uint16) {
	return 10, 61
}

func (msg *ConnectionUnblocked) Wait() bool {
	return false
}

func (msg *ConnectionUnblocked) Write(w io.Writer) (err error) {

	return
}

func (msg *ConnectionUnblocked) Read(r io.Reader) (err error) {

	return
}

type ConnectionUpdateSecret struct {
	NewSecret string
	Reason    string
}

func (msg *ConnectionUpdateSecret) ID() (uint16, uint16) {
	return 10, 70
}

func (msg *ConnectionUpdateSecret) Wait() bool {
	return true
}

func (msg *ConnectionUpdateSecret) Write(w io.Writer) (err error) {

	if err = writeLongstr(w, msg.NewSecret); err != nil {
		return
	}

	if err = writeShortstr(w, msg.Reason); err != nil {
		return
	}

	return
}

func (msg *ConnectionUpdateSecret) Read(r io.Reader) (err error) {

	if msg.NewSecret, err = readLongstr(r); err != nil {
		return
	}

	if msg.Reason, err = readShortstr(r); err != nil {
		return
	}

	return
}

type ConnectionUpdateSecretOk struct {
}

func (msg *ConnectionUpdateSecretOk) ID() (uint16, uint16) {
	return 10, 71
}

func (msg *ConnectionUpdateSecretOk) Wait() bool {
	return true
}

func (msg *ConnectionUpdateSecretOk) Write(w io.Writer) (err error) {

	return
}

func (msg *ConnectionUpdateSecretOk) Read(r io.Reader) (err error) {

	return
}

type ChannelOpen struct {
	reserved1 string
}

func (msg *ChannelOpen) ID() (uint16, uint16) {
	return 20, 10
}

func (msg *ChannelOpen) Wait() bool {
	return true
}

func (msg *ChannelOpen) Write(w io.Writer) (err error) {

	if err = writeShortstr(w, msg.reserved1); err != nil {
		return
	}

	return
}

func (msg *ChannelOpen) Read(r io.Reader) (err error) {

	if msg.reserved1, err = readShortstr(r); err != nil {
		return
	}

	return
}

type ChannelOpenOk struct {
	reserved1 string
}

func (msg *ChannelOpenOk) ID() (uint16, uint16) {
	return 20, 11
}

func (msg *ChannelOpenOk) Wait() bool {
	return true
}

func (msg *ChannelOpenOk) Write(w io.Writer) (err error) {

	if err = writeLongstr(w, msg.reserved1); err != nil {
		return
	}

	return
}

func (msg *ChannelOpenOk) Read(r io.Reader) (err error) {

	if msg.reserved1, err = readLongstr(r); err != nil {
		return
	}

	return
}

type ChannelFlow struct {
	Active bool
}

func (msg *ChannelFlow) ID() (uint16, uint16) {
	return 20, 20
}

func (msg *ChannelFlow) Wait() bool {
	return true
}

func (msg *ChannelFlow) Write(w io.Writer) (err error) {
	var bits byte

	if msg.Active {
		bits |= 1 << 0
	}

	if err = binary.Write(w, binary.BigEndian, bits); err != nil {
		return
	}

	return
}

func (msg *ChannelFlow) Read(r io.Reader) (err error) {
	var bits byte

	if err = binary.Read(r, binary.BigEndian, &bits); err != nil {
		return
	}
	msg.Active = (bits&(1<<0) > 0)

	return
}

type ChannelFlowOk struct {
	Active bool
}

func (msg *ChannelFlowOk) ID() (uint16, uint16) {
	return 20, 21
}

func (msg *ChannelFlowOk) Wait() bool {
	return false
}

func (msg *ChannelFlowOk) Write(w io.Writer) (err error) {
	var bits byte

	if msg.Active {
		bits |= 1 << 0
	}

	if err = binary.Write(w, binary.BigEndian, bits); err != nil {
		return
	}

	return
}

func (msg *ChannelFlowOk) Read(r io.Reader) (err error) {
	var bits byte

	if err = binary.Read(r, binary.BigEndian, &bits); err != nil {
		return
	}
	msg.Active = (bits&(1<<0) > 0)

	return
}

type ChannelClose struct {
	ReplyCode uint16
	ReplyText string
	ClassId   uint16
	MethodId  uint16
}

func (msg *ChannelClose) ID() (uint16, uint16) {
	return 20, 40
}

func (msg *ChannelClose) Wait() bool {
	return true
}

func (msg *ChannelClose) Write(w io.Writer) (err error) {

	if err = binary.Write(w, binary.BigEndian, msg.ReplyCode); err != nil {
		return
	}

	if err = writeShortstr(w, msg.ReplyText); err != nil {
		return
	}

	if err = binary.Write(w, binary.BigEndian, msg.ClassId); err != nil {
		return
	}
	if err = binary.Write(w, binary.BigEndian, msg.MethodId); err != nil {
		return
	}

	return
}

func (msg *ChannelClose) Read(r io.Reader) (err error) {

	if err = binary.Read(r, binary.BigEndian, &msg.ReplyCode); err != nil {
		return
	}

	if msg.ReplyText, err = readShortstr(r); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &msg.ClassId); err != nil {
		return
	}
	if err = binary.Read(r, binary.BigEndian, &msg.MethodId); err != nil {
		return
	}

	return
}

type ChannelCloseOk struct {
}

func (msg *ChannelCloseOk) ID() (uint16, uint16) {
	return 20, 41
}

func (msg *ChannelCloseOk) Wait() bool {
	return true
}

func (msg *ChannelCloseOk) Write(w io.Writer) (err error) {

	return
}

func (msg *ChannelCloseOk) Read(r io.Reader) (err error) {

	return
}

type ExchangeDeclare struct {
	reserved1  uint16
	Exchange   string
	Type       string
	Passive    bool
	Durable    bool
	AutoDelete bool
	Internal   bool
	NoWait     bool
	Arguments  Table
}

func (msg *ExchangeDeclare) ID() (uint16, uint16) {
	return 40, 10
}

func (msg *ExchangeDeclare) Wait() bool {
	return true && !msg.NoWait
}

func (msg *ExchangeDeclare) Write(w io.Writer) (err error) {
	var bits byte

	if err = binary.Write(w, binary.BigEndian, msg.reserved1); err != nil {
		return
	}

	if err = writeShortstr(w, msg.Exchange); err != nil {
		return
	}
	if err = writeShortstr(w, msg.Type); err != nil {
		return
	}

	if msg.Passive {
		bits |= 1 << 0
	}

	if msg.Durable {
		bits |= 1 << 1
	}

	if msg.AutoDelete {
		bits |= 1 << 2
	}

	if msg.Internal {
		bits |= 1 << 3
	}

	if msg.NoWait {
		bits |= 1 << 4
	}

	if err = binary.Write(w, binary.BigEndian, bits); err != nil {
		return
	}

	if err = writeTable(w, msg.Arguments); err != nil {
		return
	}

	return
}

func (msg *ExchangeDeclare) Read(r io.Reader) (err error) {
	var bits byte

	if err = binary.Read(r, binary.BigEndian, &msg.reserved1); err != nil {
		return
	}

	if msg.Exchange, err = readShortstr(r); err != nil {
		return
	}
	if msg.Type, err = readShortstr(r); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &bits); err != nil {
		return
	}
	msg.Passive = (bits&(1<<0) > 0)
	msg.Durable = (bits&(1<<1) > 0)
	msg.AutoDelete = (bits&(1<<2) > 0)
	msg.Internal = (bits&(1<<3) > 0)
	msg.NoWait = (bits&(1<<4) > 0)

	if msg.Arguments, err = readTable(r); err != nil {
		return
	}

	return
}

type ExchangeDeclareOk struct {
}

func (msg *ExchangeDeclareOk) ID() (uint16, uint16) {
	return 40, 11
}

func (msg *ExchangeDeclareOk) Wait() bool {
	return true
}

func (msg *ExchangeDeclareOk) Write(w io.Writer) (err error) {

	return
}

func (msg *ExchangeDeclareOk) Read(r io.Reader) (err error) {

	return
}

type ExchangeDelete struct {
	reserved1 uint16
	Exchange  string
	IfUnused  bool
	NoWait    bool
}

func (msg *ExchangeDelete) ID() (uint16, uint16) {
	return 40, 20
}

func (msg *ExchangeDelete) Wait() bool {
	return true && !msg.NoWait
}

func (msg *ExchangeDelete) Write(w io.Writer) (err error) {
	var bits byte

	if err = binary.Write(w, binary.BigEndian, msg.reserved1); err != nil {
		return
	}

	if err = writeShortstr(w, msg.Exchange); err != nil {
		return
	}

	if msg.IfUnused {
		bits |= 1 << 0
	}

	if msg.NoWait {
		bits |= 1 << 1
	}

	if err = binary.Write(w, binary.BigEndian, bits); err != nil {
		return
	}

	return
}

func (msg *ExchangeDelete) Read(r io.Reader) (err error) {
	var bits byte

	if err = binary.Read(r, binary.BigEndian, &msg.reserved1); err != nil {
		return
	}

	if msg.Exchange, err = readShortstr(r); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &bits); err != nil {
		return
	}
	msg.IfUnused = (bits&(1<<0) > 0)
	msg.NoWait = (bits&(1<<1) > 0)

	return
}

type ExchangeDeleteOk struct {
}

func (msg *ExchangeDeleteOk) ID() (uint16, uint16) {
	return 40, 21
}

func (msg *ExchangeDeleteOk) Wait() bool {
	return true
}

func (msg *ExchangeDeleteOk) Write(w io.Writer) (err error) {

	return
}

func (msg *ExchangeDeleteOk) Read(r io.Reader) (err error) {

	return
}

type ExchangeBind struct {
	reserved1   uint16
	Destination string
	Source      string
	RoutingKey  string
	NoWait      bool
	Arguments   Table
}

func (msg *ExchangeBind) ID() (uint16, uint16) {
	return 40, 30
}

func (msg *ExchangeBind) Wait() bool {
	return true && !msg.NoWait
}

func (msg *ExchangeBind) Write(w io.Writer) (err error) {
	var bits byte

	if err = binary.Write(w, binary.BigEndian, msg.reserved1); err != nil {
		return
	}

	if err = writeShortstr(w, msg.Destination); err != nil {
		return
	}
	if err = writeShortstr(w, msg.Source); err != nil {
		return
	}
	if err = writeShortstr(w, msg.RoutingKey); err != nil {
		return
	}

	if msg.NoWait {
		bits |= 1 << 0
	}

	if err = binary.Write(w, binary.BigEndian, bits); err != nil {
		return
	}

	if err = writeTable(w, msg.Arguments); err != nil {
		return
	}

	return
}

func (msg *ExchangeBind) Read(r io.Reader) (err error) {
	var bits byte

	if err = binary.Read(r, binary.BigEndian, &msg.reserved1); err != nil {
		return
	}

	if msg.Destination, err = readShortstr(r); err != nil {
		return
	}
	if msg.Source, err = readShortstr(r); err != nil {
		return
	}
	if msg.RoutingKey, err = readShortstr(r); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &bits); err != nil {
		return
	}
	msg.NoWait = (bits&(1<<0) > 0)

	if msg.Arguments, err = readTable(r); err != nil {
		return
	}

	return
}

type ExchangeBindOk struct {
}

func (msg *ExchangeBindOk) ID() (uint16, uint16) {
	return 40, 31
}

func (msg *ExchangeBindOk) Wait() bool {
	return true
}

func (msg *ExchangeBindOk) Write(w io.Writer) (err error) {

	return
}

func (msg *ExchangeBindOk) Read(r io.Reader) (err error) {

	return
}

type ExchangeUnbind struct {
	reserved1   uint16
	Destination string
	Source      string
	RoutingKey  string
	NoWait      bool
	Arguments   Table
}

func (msg *ExchangeUnbind) ID() (uint16, uint16) {
	return 40, 40
}

func (msg *ExchangeUnbind) Wait() bool {
	return true && !msg.NoWait
}

func (msg *ExchangeUnbind) Write(w io.Writer) (err error) {
	var bits byte

	if err = binary.Write(w, binary.BigEndian, msg.reserved1); err != nil {
		return
	}

	if err = writeShortstr(w, msg.Destination); err != nil {
		return
	}
	if err = writeShortstr(w, msg.Source); err != nil {
		return
	}
	if err = writeShortstr(w, msg.RoutingKey); err != nil {
		return
	}

	if msg.NoWait {
		bits |= 1 << 0
	}

	if err = binary.Write(w, binary.BigEndian, bits); err != nil {
		return
	}

	if err = writeTable(w, msg.Arguments); err != nil {
		return
	}

	return
}

func (msg *ExchangeUnbind) Read(r io.Reader) (err error) {
	var bits byte

	if err = binary.Read(r, binary.BigEndian, &msg.reserved1); err != nil {
		return
	}

	if msg.Destination, err = readShortstr(r); err != nil {
		return
	}
	if msg.Source, err = readShortstr(r); err != nil {
		return
	}
	if msg.RoutingKey, err = readShortstr(r); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &bits); err != nil {
		return
	}
	msg.NoWait = (bits&(1<<0) > 0)

	if msg.Arguments, err = readTable(r); err != nil {
		return
	}

	return
}

type ExchangeUnbindOk struct {
}

func (msg *ExchangeUnbindOk) ID() (uint16, uint16) {
	return 40, 51
}

func (msg *ExchangeUnbindOk) Wait() bool {
	return true
}

func (msg *ExchangeUnbindOk) Write(w io.Writer) (err error) {

	return
}

func (msg *ExchangeUnbindOk) Read(r io.Reader) (err error) {

	return
}

type QueueDeclare struct {
	reserved1  uint16
	Queue      string
	Passive    bool
	Durable    bool
	Exclusive  bool
	AutoDelete bool
	NoWait     bool
	Arguments  Table
}

func (msg *QueueDeclare) ID() (uint16, uint16) {
	return 50, 10
}

func (msg *QueueDeclare) Wait() bool {
	return true && !msg.NoWait
}

func (msg *QueueDeclare) Write(w io.Writer) (err error) {
	var bits byte

	if err = binary.Write(w, binary.BigEndian, msg.reserved1); err != nil {
		return
	}

	if err = writeShortstr(w, msg.Queue); err != nil {
		return
	}

	if msg.Passive {
		bits |= 1 << 0
	}

	if msg.Durable {
		bits |= 1 << 1
	}

	if msg.Exclusive {
		bits |= 1 << 2
	}

	if msg.AutoDelete {
		bits |= 1 << 3
	}

	if msg.NoWait {
		bits |= 1 << 4
	}

	if err = binary.Write(w, binary.BigEndian, bits); err != nil {
		return
	}

	if err = writeTable(w, msg.Arguments); err != nil {
		return
	}

	return
}

func (msg *QueueDeclare) Read(r io.Reader) (err error) {
	var bits byte

	if err = binary.Read(r, binary.BigEndian, &msg.reserved1); err != nil {
		return
	}

	if msg.Queue, err = readShortstr(r); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &bits); err != nil {
		return
	}
	msg.Passive = (bits&(1<<0) > 0)
	msg.Durable = (bits&(1<<1) > 0)
	msg.Exclusive = (bits&(1<<2) > 0)
	msg.AutoDelete = (bits&(1<<3) > 0)
	msg.NoWait = (bits&(1<<4) > 0)

	if msg.Arguments, err = readTable(r); err != nil {
		return
	}

	return
}

type QueueDeclareOk struct {
	Queue         string
	MessageCount  uint32
	ConsumerCount uint32
}

func (msg *QueueDeclareOk) ID() (uint16, uint16) {
	return 50, 11
}

func (msg *QueueDeclareOk) Wait() bool {
	return true
}

func (msg *QueueDeclareOk) Write(w io.Writer) (err error) {

	if err = writeShortstr(w, msg.Queue); err != nil {
		return
	}

	if err = binary.Write(w, binary.BigEndian, msg.MessageCount); err != nil {
		return
	}
	if err = binary.Write(w, binary.BigEndian, msg.ConsumerCount); err != nil {
		return
	}

	return
}

func (msg *QueueDeclareOk) Read(r io.Reader) (err error) {

	if msg.Queue, err = readShortstr(r); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &msg.MessageCount); err != nil {
		return
	}
	if err = binary.Read(r, binary.BigEndian, &msg.ConsumerCount); err != nil {
		return
	}

	return
}

type QueueBind struct {
	reserved1  uint16
	Queue      string
	Exchange   string
	RoutingKey string
	NoWait     bool
	Arguments  Table
}

func (msg *QueueBind) ID() (uint16, uint16) {
	return 50, 20
}

func (msg *QueueBind) Wait() bool {
	return true && !msg.NoWait
}

func (msg *QueueBind) Write(w io.Writer) (err error) {
	var bits byte

	if err = binary.Write(w, binary.BigEndian, msg.reserved1); err != nil {
		return
	}

	if err = writeShortstr(w, msg.Queue); err != nil {
		return
	}
	if err = writeShortstr(w, msg.Exchange); err != nil {
		return
	}
	if err = writeShortstr(w, msg.RoutingKey); err != nil {
		return
	}

	if msg.NoWait {
		bits |= 1 << 0
	}

	if err = binary.Write(w, binary.BigEndian, bits); err != nil {
		return
	}

	if err = writeTable(w, msg.Arguments); err != nil {
		return
	}

	return
}

func (msg *QueueBind) Read(r io.Reader) (err error) {
	var bits byte

	if err = binary.Read(r, binary.BigEndian, &msg.reserved1); err != nil {
		return
	}

	if msg.Queue, err = readShortstr(r); err != nil {
		return
	}
	if msg.Exchange, err = readShortstr(r); err != nil {
		return
	}
	if msg.RoutingKey, err = readShortstr(r); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &bits); err != nil {
		return
	}
	msg.NoWait = (bits&(1<<0) > 0)

	if msg.Arguments, err = readTable(r); err != nil {
		return
	}

	return
}

type QueueBindOk struct {
}

func (msg *QueueBindOk) ID() (uint16, uint16) {
	return 50, 21
}

func (msg *QueueBindOk) Wait() bool {
	return true
}

func (msg *QueueBindOk) Write(w io.Writer) (err error) {

	return
}

func (msg *QueueBindOk) Read(r io.Reader) (err error) {

	return
}

type QueueUnbind struct {
	reserved1  uint16
	Queue      string
	Exchange   string
	RoutingKey string
	Arguments  Table
}

func (msg *QueueUnbind) ID() (uint16, uint16) {
	return 50, 50
}

func (msg *QueueUnbind) Wait() bool {
	return true
}

func (msg *QueueUnbind) Write(w io.Writer) (err error) {

	if err = binary.Write(w, binary.BigEndian, msg.reserved1); err != nil {
		return
	}

	if err = writeShortstr(w, msg.Queue); err != nil {
		return
	}
	if err = writeShortstr(w, msg.Exchange); err != nil {
		return
	}
	if err = writeShortstr(w, msg.RoutingKey); err != nil {
		return
	}

	if err = writeTable(w, msg.Arguments); err != nil {
		return
	}

	return
}

func (msg *QueueUnbind) Read(r io.Reader) (err error) {

	if err = binary.Read(r, binary.BigEndian, &msg.reserved1); err != nil {
		return
	}

	if msg.Queue, err = readShortstr(r); err != nil {
		return
	}
	if msg.Exchange, err = readShortstr(r); err != nil {
		return
	}
	if msg.RoutingKey, err = readShortstr(r); err != nil {
		return
	}

	if msg.Arguments, err = readTable(r); err != nil {
		return
	}

	return
}

type QueueUnbindOk struct {
}

func (msg *QueueUnbindOk) ID() (uint16, uint16) {
	return 50, 51
}

func (msg *QueueUnbindOk) Wait() bool {
	return true
}

func (msg *QueueUnbindOk) Write(w io.Writer) (err error) {

	return
}

func (msg *QueueUnbindOk) Read(r io.Reader) (err error) {

	return
}

type QueuePurge struct {
	reserved1 uint16
	Queue     string
	NoWait    bool
}

func (msg *QueuePurge) ID() (uint16, uint16) {
	return 50, 30
}

func (msg *QueuePurge) Wait() bool {
	return true && !msg.NoWait
}

func (msg *QueuePurge) Write(w io.Writer) (err error) {
	var bits byte

	if err = binary.Write(w, binary.BigEndian, msg.reserved1); err != nil {
		return
	}

	if err = writeShortstr(w, msg.Queue); err != nil {
		return
	}

	if msg.NoWait {
		bits |= 1 << 0
	}

	if err = binary.Write(w, binary.BigEndian, bits); err != nil {
		return
	}

	return
}

func (msg *QueuePurge) Read(r io.Reader) (err error) {
	var bits byte

	if err = binary.Read(r, binary.BigEndian, &msg.reserved1); err != nil {
		return
	}

	if msg.Queue, err = readShortstr(r); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &bits); err != nil {
		return
	}
	msg.NoWait = (bits&(1<<0) > 0)

	return
}

type QueuePurgeOk struct {
	MessageCount uint32
}

func (msg *QueuePurgeOk) ID() (uint16, uint16) {
	return 50, 31
}

func (msg *QueuePurgeOk) Wait() bool {
	return true
}

func (msg *QueuePurgeOk) Write(w io.Writer) (err error) {

	if err = binary.Write(w, binary.BigEndian, msg.MessageCount); err != nil {
		return
	}

	return
}

func (msg *QueuePurgeOk) Read(r io.Reader) (err error) {

	if err = binary.Read(r, binary.BigEndian, &msg.MessageCount); err != nil {
		return
	}

	return
}

type QueueDelete struct {
	reserved1 uint16
	Queue     string
	IfUnused  bool
	IfEmpty   bool
	NoWait    bool
}

func (msg *QueueDelete) ID() (uint16, uint16) {
	return 50, 40
}

func (msg *QueueDelete) Wait() bool {
	return true && !msg.NoWait
}

func (msg *QueueDelete) Write(w io.Writer) (err error) {
	var bits byte

	if err = binary.Write(w, binary.BigEndian, msg.reserved1); err != nil {
		return
	}

	if err = writeShortstr(w, msg.Queue); err != nil {
		return
	}

	if msg.IfUnused {
		bits |= 1 << 0
	}

	if msg.IfEmpty {
		bits |= 1 << 1
	}

	if msg.NoWait {
		bits |= 1 << 2
	}

	if err = binary.Write(w, binary.BigEndian, bits); err != nil {
		return
	}

	return
}

func (msg *QueueDelete) Read(r io.Reader) (err error) {
	var bits byte

	if err = binary.Read(r, binary.BigEndian, &msg.reserved1); err != nil {
		return
	}

	if msg.Queue, err = readShortstr(r); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &bits); err != nil {
		return
	}
	msg.IfUnused = (bits&(1<<0) > 0)
	msg.IfEmpty = (bits&(1<<1) > 0)
	msg.NoWait = (bits&(1<<2) > 0)

	return
}

type QueueDeleteOk struct {
	MessageCount uint32
}

func (msg *QueueDeleteOk) ID() (uint16, uint16) {
	return 50, 41
}

func (msg *QueueDeleteOk) Wait() bool {
	return true
}

func (msg *QueueDeleteOk) Write(w io.Writer) (err error) {

	if err = binary.Write(w, binary.BigEndian, msg.MessageCount); err != nil {
		return
	}

	return
}

func (msg *QueueDeleteOk) Read(r io.Reader) (err error) {

	if err = binary.Read(r, binary.BigEndian, &msg.MessageCount); err != nil {
		return
	}

	return
}

type BasicQos struct {
	PrefetchSize  uint32
	PrefetchCount uint16
	Global        bool
}

func (msg *BasicQos) ID() (uint16, uint16) {
	return 60, 10
}

func (msg *BasicQos) Wait() bool {
	return true
}

func (msg *BasicQos) Write(w io.Writer) (err error) {
	var bits byte

	if err = binary.Write(w, binary.BigEndian, msg.PrefetchSize); err != nil {
		return
	}

	if err = binary.Write(w, binary.BigEndian, msg.PrefetchCount); err != nil {
		return
	}

	if msg.Global {
		bits |= 1 << 0
	}

	if err = binary.Write(w, binary.BigEndian, bits); err != nil {
		return
	}

	return
}

func (msg *BasicQos) Read(r io.Reader) (err error) {
	var bits byte

	if err = binary.Read(r, binary.BigEndian, &msg.PrefetchSize); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &msg.PrefetchCount); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &bits); err != nil {
		return
	}
	msg.Global = (bits&(1<<0) > 0)

	return
}

type BasicQosOk struct {
}

func (msg *BasicQosOk) ID() (uint16, uint16) {
	return 60, 11
}

func (msg *BasicQosOk) Wait() bool {
	return true
}

func (msg *BasicQosOk) Write(w io.Writer) (err error) {

	return
}

func (msg *BasicQosOk) Read(r io.Reader) (err error) {

	return
}

type BasicConsume struct {
	reserved1   uint16
	Queue       string
	ConsumerTag string
	NoLocal     bool
	NoAck       bool
	Exclusive   bool
	NoWait      bool
	Arguments   Table
}

func (msg *BasicConsume) ID() (uint16, uint16) {
	return 60, 20
}

func (msg *BasicConsume) Wait() bool {
	return true && !msg.NoWait
}

func (msg *BasicConsume) Write(w io.Writer) (err error) {
	var bits byte

	if err = binary.Write(w, binary.BigEndian, msg.reserved1); err != nil {
		return
	}

	if err = writeShortstr(w, msg.Queue); err != nil {
		return
	}
	if err = writeShortstr(w, msg.ConsumerTag); err != nil {
		return
	}

	if msg.NoLocal {
		bits |= 1 << 0
	}

	if msg.NoAck {
		bits |= 1 << 1
	}

	if msg.Exclusive {
		bits |= 1 << 2
	}

	if msg.NoWait {
		bits |= 1 << 3
	}

	if err = binary.Write(w, binary.BigEndian, bits); err != nil {
		return
	}

	if err = writeTable(w, msg.Arguments); err != nil {
		return
	}

	return
}

func (msg *BasicConsume) Read(r io.Reader) (err error) {
	var bits byte

	if err = binary.Read(r, binary.BigEndian, &msg.reserved1); err != nil {
		return
	}

	if msg.Queue, err = readShortstr(r); err != nil {
		return
	}
	if msg.ConsumerTag, err = readShortstr(r); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &bits); err != nil {
		return
	}
	msg.NoLocal = (bits&(1<<0) > 0)
	msg.NoAck = (bits&(1<<1) > 0)
	msg.Exclusive = (bits&(1<<2) > 0)
	msg.NoWait = (bits&(1<<3) > 0)

	if msg.Arguments, err = readTable(r); err != nil {
		return
	}

	return
}

type BasicConsumeOk struct {
	ConsumerTag string
}

func (msg *BasicConsumeOk) ID() (uint16, uint16) {
	return 60, 21
}

func (msg *BasicConsumeOk) Wait() bool {
	return true
}

func (msg *BasicConsumeOk) Write(w io.Writer) (err error) {

	if err = writeShortstr(w, msg.ConsumerTag); err != nil {
		return
	}

	return
}

func (msg *BasicConsumeOk) Read(r io.Reader) (err error) {

	if msg.ConsumerTag, err = readShortstr(r); err != nil {
		return
	}

	return
}

type BasicCancel struct {
	ConsumerTag string
	NoWait      bool
}

func (msg *BasicCancel) ID() (uint16, uint16) {
	return 60, 30
}

func (msg *BasicCancel) Wait() bool {
	return true && !msg.NoWait
}

func (msg *BasicCancel) Write(w io.Writer) (err error) {
	var bits byte

	if err = writeShortstr(w, msg.ConsumerTag); err != nil {
		return
	}

	if msg.NoWait {
		bits |= 1 << 0
	}

	if err = binary.Write(w, binary.BigEndian, bits); err != nil {
		return
	}

	return
}

func (msg *BasicCancel) Read(r io.Reader) (err error) {
	var bits byte

	if msg.ConsumerTag, err = readShortstr(r); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &bits); err != nil {
		return
	}
	msg.NoWait = (bits&(1<<0) > 0)

	return
}

type BasicCancelOk struct {
	ConsumerTag string
}

func (msg *BasicCancelOk) ID() (uint16, uint16) {
	return 60, 31
}

func (msg *BasicCancelOk) Wait() bool {
	return true
}

func (msg *BasicCancelOk) Write(w io.Writer) (err error) {

	if err = writeShortstr(w, msg.ConsumerTag); err != nil {
		return
	}

	return
}

func (msg *BasicCancelOk) Read(r io.Reader) (err error) {

	if msg.ConsumerTag, err = readShortstr(r); err != nil {
		return
	}

	return
}

type BasicPublish struct {
	reserved1  uint16
	Exchange   string
	RoutingKey string
	Mandatory  bool
	Immediate  bool
	Properties Properties
	Body       []byte
}

func (msg *BasicPublish) ID() (uint16, uint16) {
	return 60, 40
}

func (msg *BasicPublish) Wait() bool {
	return false
}

func (msg *BasicPublish) GetContent() (Properties, []byte) {
	return msg.Properties, msg.Body
}

func (msg *BasicPublish) SetContent(props Properties, body []byte) {
	msg.Properties, msg.Body = props, body
}

func (msg *BasicPublish) Write(w io.Writer) (err error) {
	var bits byte

	if err = binary.Write(w, binary.BigEndian, msg.reserved1); err != nil {
		return
	}

	if err = writeShortstr(w, msg.Exchange); err != nil {
		return
	}
	if err = writeShortstr(w, msg.RoutingKey); err != nil {
		return
	}

	if msg.Mandatory {
		bits |= 1 << 0
	}

	if msg.Immediate {
		bits |= 1 << 1
	}

	if err = binary.Write(w, binary.BigEndian, bits); err != nil {
		return
	}

	return
}

func (msg *BasicPublish) Read(r io.Reader) (err error) {
	var bits byte

	if err = binary.Read(r, binary.BigEndian, &msg.reserved1); err != nil {
		return
	}

	if msg.Exchange, err = readShortstr(r); err != nil {
		return
	}
	if msg.RoutingKey, err = readShortstr(r); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &bits); err != nil {
		return
	}
	msg.Mandatory = (bits&(1<<0) > 0)
	msg.Immediate = (bits&(1<<1) > 0)

	return
}

type BasicReturn struct {
	ReplyCode  uint16
	ReplyText  string
	Exchange   string
	RoutingKey string
	Properties Properties
	Body       []byte
}

func (msg *BasicReturn) ID() (uint16, uint16) {
	return 60, 50
}

func (msg *BasicReturn) Wait() bool {
	return false
}

func (msg *BasicReturn) GetContent() (Properties, []byte) {
	return msg.Properties, msg.Body
}

func (msg *BasicReturn) SetContent(props Properties, body []byte) {
	msg.Properties, msg.Body = props, body
}

func (msg *BasicReturn) Write(w io.Writer) (err error) {

	if err = binary.Write(w, binary.BigEndian, msg.ReplyCode); err != nil {
		return
	}

	if err = writeShortstr(w, msg.ReplyText); err != nil {
		return
	}
	if err = writeShortstr(w, msg.Exchange); err != nil {
		return
	}
	if err = writeShortstr(w, msg.RoutingKey); err != nil {
		return
	}

	return
}

func (msg *BasicReturn) Read(r io.Reader) (err error) {

	if err = binary.Read(r, binary.BigEndian, &msg.ReplyCode); err != nil {
		return
	}

	if msg.ReplyText, err = readShortstr(r); err != nil {
		return
	}
	if msg.Exchange, err = readShortstr(r); err != nil {
		return
	}
	if msg.RoutingKey, err = readShortstr(r); err != nil {
		return
	}

	return
}

type BasicDeliver struct {
	ConsumerTag string
	DeliveryTag uint64
	Redelivered bool
	Exchange    string
	RoutingKey  string
	Properties  Properties
	Body        []byte
}

func (msg *BasicDeliver) ID() (uint16, uint16) {
	return 60, 60
}

func (msg *BasicDeliver) Wait() bool {
	return false
}

func (msg *BasicDeliver) GetContent() (Properties, []byte) {
	return msg.Properties, msg.Body
}

func (msg *BasicDeliver) SetContent(props Properties, body []byte) {
	msg.Properties, msg.Body = props, body
}

func (msg *BasicDeliver) Write(w io.Writer) (err error) {
	var bits byte

	if err = writeShortstr(w, msg.ConsumerTag); err != nil {
		return
	}

	if err = binary.Write(w, binary.BigEndian, msg.DeliveryTag); err != nil {
		return
	}

	if msg.Redelivered {
		bits |= 1 << 0
	}

	if err = binary.Write(w, binary.BigEndian, bits); err != nil {
		return
	}

	if err = writeShortstr(w, msg.Exchange); err != nil {
		return
	}
	if err = writeShortstr(w, msg.RoutingKey); err != nil {
		return
	}

	return
}

func (msg *BasicDeliver) Read(r io.Reader) (err error) {
	var bits byte

	if msg.ConsumerTag, err = readShortstr(r); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &msg.DeliveryTag); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &bits); err != nil {
		return
	}
	msg.Redelivered = (bits&(1<<0) > 0)

	if msg.Exchange, err = readShortstr(r); err != nil {
		return
	}
	if msg.RoutingKey, err = readShortstr(r); err != nil {
		return
	}

	return
}

type BasicGet struct {
	reserved1 uint16
	Queue     string
	NoAck     bool
}

func (msg *BasicGet) ID() (uint16, uint16) {
	return 60, 70
}

func (msg *BasicGet) Wait() bool {
	return true
}

func (msg *BasicGet) Write(w io.Writer) (err error) {
	var bits byte

	if err = binary.Write(w, binary.BigEndian, msg.reserved1); err != nil {
		return
	}

	if err = writeShortstr(w, msg.Queue); err != nil {
		return
	}

	if msg.NoAck {
		bits |= 1 << 0
	}

	if err = binary.Write(w, binary.BigEndian, bits); err != nil {
		return
	}

	return
}

func (msg *BasicGet) Read(r io.Reader) (err error) {
	var bits byte

	if err = binary.Read(r, binary.BigEndian, &msg.reserved1); err != nil {
		return
	}

	if msg.Queue, err = readShortstr(r); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &bits); err != nil {
		return
	}
	msg.NoAck = (bits&(1<<0) > 0)

	return
}

type BasicGetOk struct {
	DeliveryTag  uint64
	Redelivered  bool
	Exchange     string
	RoutingKey   string
	MessageCount uint32
	Properties   Properties
	Body         []byte
}

func (msg *BasicGetOk) ID() (uint16, uint16) {
	return 60, 71
}

func (msg *BasicGetOk) Wait() bool {
	return true
}

func (msg *BasicGetOk) GetContent() (Properties, []byte) {
	return msg.Properties, msg.Body
}

func (msg *BasicGetOk) SetContent(props Properties, body []byte) {
	msg.Properties, msg.Body = props, body
}

func (msg *BasicGetOk) Write(w io.Writer) (err error) {
	var bits byte

	if err = binary.Write(w, binary.BigEndian, msg.DeliveryTag); err != nil {
		return
	}

	if msg.Redelivered {
		bits |= 1 << 0
	}

	if err = binary.Write(w, binary.BigEndian, bits); err != nil {
		return
	}

	if err = writeShortstr(w, msg.Exchange); err != nil {
		return
	}
	if err = writeShortstr(w, msg.RoutingKey); err != nil {
		return
	}

	if err = binary.Write(w, binary.BigEndian, msg.MessageCount); err != nil {
		return
	}

	return
}

func (msg *BasicGetOk) Read(r io.Reader) (err error) {
	var bits byte

	if err = binary.Read(r, binary.BigEndian, &msg.DeliveryTag); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &bits); err != nil {
		return
	}
	msg.Redelivered = (bits&(1<<0) > 0)

	if msg.Exchange, err = readShortstr(r); err != nil {
		return
	}
	if msg.RoutingKey, err = readShortstr(r); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &msg.MessageCount); err != nil {
		return
	}

	return
}

type BasicGetEmpty struct {
	reserved1 string
}

func (msg *BasicGetEmpty) ID() (uint16, uint16) {
	return 60, 72
}

func (msg *BasicGetEmpty) Wait() bool {
	return true
}

func (msg *BasicGetEmpty) Write(w io.Writer) (err error) {

	if err = writeShortstr(w, msg.reserved1); err != nil {
		return
	}

	return
}

func (msg *BasicGetEmpty) Read(r io.Reader) (err error) {

	if msg.reserved1, err = readShortstr(r); err != nil {
		return
	}

	return
}

type BasicAck struct {
	DeliveryTag uint64
	Multiple    bool
}

func (msg *BasicAck) ID() (uint16, uint16) {
	return 60, 80
}

func (msg *BasicAck) Wait() bool {
	return false
}

func (msg *BasicAck) Write(w io.Writer) (err error) {
	var bits byte

	if err = binary.Write(w, binary.BigEndian, msg.DeliveryTag); err != nil {
		return
	}

	if msg.Multiple {
		bits |= 1 << 0
	}

	if err = binary.Write(w, binary.BigEndian, bits); err != nil {
		return
	}

	return
}

func (msg *BasicAck) Read(r io.Reader) (err error) {
	var bits byte

	if err = binary.Read(r, binary.BigEndian, &msg.DeliveryTag); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &bits); err != nil {
		return
	}
	msg.Multiple = (bits&(1<<0) > 0)

	return
}

type BasicReject struct {
	DeliveryTag uint64
	Requeue     bool
}

func (msg *BasicReject) ID() (uint16, uint16) {
	return 60, 90
}

func (msg *BasicReject) Wait() bool {
	return false
}

func (msg *BasicReject) Write(w io.Writer) (err error) {
	var bits byte

	if err = binary.Write(w, binary.BigEndian, msg.DeliveryTag); err != nil {
		return
	}

	if msg.Requeue {
		bits |= 1 << 0
	}

	if err = binary.Write(w, binary.BigEndian, bits); err != nil {
		return
	}

	return
}

func (msg *BasicReject) Read(r io.Reader) (err error) {
	var bits byte

	if err = binary.Read(r, binary.BigEndian, &msg.DeliveryTag); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &bits); err != nil {
		return
	}
	msg.Requeue = (bits&(1<<0) > 0)

	return
}

type BasicRecoverAsync struct {
	Requeue bool
}

func (msg *BasicRecoverAsync) ID() (uint16, uint16) {
	return 60, 100
}

func (msg *BasicRecoverAsync) Wait() bool {
	return false
}

func (msg *BasicRecoverAsync) Write(w io.Writer) (err error) {
	var bits byte

	if msg.Requeue {
		bits |= 1 << 0
	}

	if err = binary.Write(w, binary.BigEndian, bits); err != nil {
		return
	}

	return
}

func (msg *BasicRecoverAsync) Read(r io.Reader) (err error) {
	var bits byte

	if err = binary.Read(r, binary.BigEndian, &bits); err != nil {
		return
	}
	msg.Requeue = (bits&(1<<0) > 0)

	return
}

type BasicRecover struct {
	Requeue bool
}

func (msg *BasicRecover) ID() (uint16, uint16) {
	return 60, 110
}

func (msg *BasicRecover) Wait() bool {
	return true
}

func (msg *BasicRecover) Write(w io.Writer) (err error) {
	var bits byte

	if msg.Requeue {
		bits |= 1 << 0
	}

	if err = binary.Write(w, binary.BigEndian, bits); err != nil {
		return
	}

	return
}

func (msg *BasicRecover) Read(r io.Reader) (err error) {
	var bits byte

	if err = binary.Read(r, binary.BigEndian, &bits); err != nil {
		return
	}
	msg.Requeue = (bits&(1<<0) > 0)

	return
}

type BasicRecoverOk struct {
}

func (msg *BasicRecoverOk) ID() (uint16, uint16) {
	return 60, 111
}

func (msg *BasicRecoverOk) Wait() bool {
	return true
}

func (msg *BasicRecoverOk) Write(w io.Writer) (err error) {

	return
}

func (msg *BasicRecoverOk) Read(r io.Reader) (err error) {

	return
}

type BasicNack struct {
	DeliveryTag uint64
	Multiple    bool
	Requeue     bool
}

func (msg *BasicNack) ID() (uint16, uint16) {
	return 60, 120
}

func (msg *BasicNack) Wait() bool {
	return false
}

func (msg *BasicNack) Write(w io.Writer) (err error) {
	var bits byte

	if err = binary.Write(w, binary.BigEndian, msg.DeliveryTag); err != nil {
		return
	}

	if msg.Multiple {
		bits |= 1 << 0
	}

	if msg.Requeue {
		bits |= 1 << 1
	}

	if err = binary.Write(w, binary.BigEndian, bits); err != nil {
		return
	}

	return
}

func (msg *BasicNack) Read(r io.Reader) (err error) {
	var bits byte

	if err = binary.Read(r, binary.BigEndian, &msg.DeliveryTag); err != nil {
		return
	}

	if err = binary.Read(r, binary.BigEndian, &bits); err != nil {
		return
	}
	msg.Multiple = (bits&(1<<0) > 0)
	msg.Requeue = (bits&(1<<1) > 0)

	return
}

type TxSelect struct {
}

func (msg *TxSelect) ID() (uint16, uint16) {
	return 90, 10
}

func (msg *TxSelect) Wait() bool {
	return true
}

func (msg *TxSelect) Write(w io.Writer) (err error) {

	return
}

func (msg *TxSelect) Read(r io.Reader) (err error) {

	return
}

type TxSelectOk struct {
}

func (msg *TxSelectOk) ID() (uint16, uint16) {
	return 90, 11
}

func (msg *TxSelectOk) Wait() bool {
	return true
}

func (msg *TxSelectOk) Write(w io.Writer) (err error) {

	return
}

func (msg *TxSelectOk) Read(r io.Reader) (err error) {

	return
}

type TxCommit struct {
}

func (msg *TxCommit) ID() (uint16, uint16) {
	return 90, 20
}

func (msg *TxCommit) Wait() bool {
	return true
}

func (msg *TxCommit) Write(w io.Writer) (err error) {

	return
}

func (msg *TxCommit) Read(r io.Reader) (err error) {

	return
}

type TxCommitOk struct {
}

func (msg *TxCommitOk) ID() (uint16, uint16) {
	return 90, 21
}

func (msg *TxCommitOk) Wait() bool {
	return true
}

func (msg *TxCommitOk) Write(w io.Writer) (err error) {

	return
}

func (msg *TxCommitOk) Read(r io.Reader) (err error) {

	return
}

type TxRollback struct {
}

func (msg *TxRollback) ID() (uint16, uint16) {
	return 90, 30
}

func (msg *TxRollback) Wait() bool {
	return true
}

func (msg *TxRollback) Write(w io.Writer) (err error) {

	return
}

func (msg *TxRollback) Read(r io.Reader) (err error) {

	return
}

type TxRollbackOk struct {
}

func (msg *TxRollbackOk) ID() (uint16, uint16) {
	return 90, 31
}

func (msg *TxRollbackOk) Wait() bool {
	return true
}

func (msg *TxRollbackOk) Write(w io.Writer) (err error) {

	return
}

func (msg *TxRollbackOk) Read(r io.Reader) (err error) {

	return
}

type ConfirmSelect struct {
	Nowait bool
}

func (msg *ConfirmSelect) ID() (uint16, uint16) {
	return 85, 10
}

func (msg *ConfirmSelect) Wait() bool {
	return true
}

func (msg *ConfirmSelect) Write(w io.Writer) (err error) {
	var bits byte

	if msg.Nowait {
		bits |= 1 << 0
	}

	if err = binary.Write(w, binary.BigEndian, bits); err != nil {
		return
	}

	return
}

func (msg *ConfirmSelect) Read(r io.Reader) (err error) {
	var bits byte

	if err = binary.Read(r, binary.BigEndian, &bits); err != nil {
		return
	}
	msg.Nowait = (bits&(1<<0) > 0)

	return
}

type ConfirmSelectOk struct {
}

func (msg *ConfirmSelectOk) ID() (uint16, uint16) {
	return 85, 11
}

func (msg *ConfirmSelectOk) Wait() bool {
	return true
}

func (msg *ConfirmSelectOk) Write(w io.Writer) (err error) {

	return
}

func (msg *ConfirmSelectOk) Read(r io.Reader) (err error) {

	return
}

func (r *Reader) ParseMethodFrame(channel uint16, size uint32) (f Frame, err error) {
	mf := &MethodFrame{
		ChannelId: channel,
	}

	if err = binary.Read(r.r, binary.BigEndian, &mf.ClassId); err != nil {
		return
	}

	if err = binary.Read(r.r, binary.BigEndian, &mf.MethodId); err != nil {
		return
	}

	switch mf.ClassId {

	case 10: // connection
		switch mf.MethodId {

		case 10: // connection start
			// fmt.Println("NextMethod: class:10 method:10")
			method := &ConnectionStart{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 11: // connection start-ok
			// fmt.Println("NextMethod: class:10 method:11")
			method := &ConnectionStartOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 20: // connection secure
			// fmt.Println("NextMethod: class:10 method:20")
			method := &ConnectionSecure{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 21: // connection secure-ok
			// fmt.Println("NextMethod: class:10 method:21")
			method := &ConnectionSecureOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 30: // connection tune
			// fmt.Println("NextMethod: class:10 method:30")
			method := &ConnectionTune{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 31: // connection tune-ok
			// fmt.Println("NextMethod: class:10 method:31")
			method := &ConnectionTuneOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 40: // connection open
			// fmt.Println("NextMethod: class:10 method:40")
			method := &ConnectionOpen{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 41: // connection open-ok
			// fmt.Println("NextMethod: class:10 method:41")
			method := &ConnectionOpenOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 50: // connection close
			// fmt.Println("NextMethod: class:10 method:50")
			method := &ConnectionClose{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 51: // connection close-ok
			// fmt.Println("NextMethod: class:10 method:51")
			method := &ConnectionCloseOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 60: // connection blocked
			// fmt.Println("NextMethod: class:10 method:60")
			method := &ConnectionBlocked{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 61: // connection unblocked
			// fmt.Println("NextMethod: class:10 method:61")
			method := &ConnectionUnblocked{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 70: // connection update-secret
			// fmt.Println("NextMethod: class:10 method:70")
			method := &ConnectionUpdateSecret{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 71: // connection update-secret-ok
			// fmt.Println("NextMethod: class:10 method:71")
			method := &ConnectionUpdateSecretOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		default:
			return nil, fmt.Errorf("Bad method frame, unknown method %d for class %d", mf.MethodId, mf.ClassId)
		}

	case 20: // channel
		switch mf.MethodId {

		case 10: // channel open
			// fmt.Println("NextMethod: class:20 method:10")
			method := &ChannelOpen{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 11: // channel open-ok
			// fmt.Println("NextMethod: class:20 method:11")
			method := &ChannelOpenOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 20: // channel flow
			// fmt.Println("NextMethod: class:20 method:20")
			method := &ChannelFlow{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 21: // channel flow-ok
			// fmt.Println("NextMethod: class:20 method:21")
			method := &ChannelFlowOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 40: // channel close
			// fmt.Println("NextMethod: class:20 method:40")
			method := &ChannelClose{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 41: // channel close-ok
			// fmt.Println("NextMethod: class:20 method:41")
			method := &ChannelCloseOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		default:
			return nil, fmt.Errorf("Bad method frame, unknown method %d for class %d", mf.MethodId, mf.ClassId)
		}

	case 40: // exchange
		switch mf.MethodId {

		case 10: // exchange declare
			// fmt.Println("NextMethod: class:40 method:10")
			method := &ExchangeDeclare{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 11: // exchange declare-ok
			// fmt.Println("NextMethod: class:40 method:11")
			method := &ExchangeDeclareOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 20: // exchange delete
			// fmt.Println("NextMethod: class:40 method:20")
			method := &ExchangeDelete{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 21: // exchange delete-ok
			// fmt.Println("NextMethod: class:40 method:21")
			method := &ExchangeDeleteOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 30: // exchange bind
			// fmt.Println("NextMethod: class:40 method:30")
			method := &ExchangeBind{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 31: // exchange bind-ok
			// fmt.Println("NextMethod: class:40 method:31")
			method := &ExchangeBindOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 40: // exchange unbind
			// fmt.Println("NextMethod: class:40 method:40")
			method := &ExchangeUnbind{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 51: // exchange unbind-ok
			// fmt.Println("NextMethod: class:40 method:51")
			method := &ExchangeUnbindOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		default:
			return nil, fmt.Errorf("Bad method frame, unknown method %d for class %d", mf.MethodId, mf.ClassId)
		}

	case 50: // queue
		switch mf.MethodId {

		case 10: // queue declare
			// fmt.Println("NextMethod: class:50 method:10")
			method := &QueueDeclare{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 11: // queue declare-ok
			// fmt.Println("NextMethod: class:50 method:11")
			method := &QueueDeclareOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 20: // queue bind
			// fmt.Println("NextMethod: class:50 method:20")
			method := &QueueBind{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 21: // queue bind-ok
			// fmt.Println("NextMethod: class:50 method:21")
			method := &QueueBindOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 50: // queue unbind
			// fmt.Println("NextMethod: class:50 method:50")
			method := &QueueUnbind{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 51: // queue unbind-ok
			// fmt.Println("NextMethod: class:50 method:51")
			method := &QueueUnbindOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 30: // queue purge
			// fmt.Println("NextMethod: class:50 method:30")
			method := &QueuePurge{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 31: // queue purge-ok
			// fmt.Println("NextMethod: class:50 method:31")
			method := &QueuePurgeOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 40: // queue delete
			// fmt.Println("NextMethod: class:50 method:40")
			method := &QueueDelete{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 41: // queue delete-ok
			// fmt.Println("NextMethod: class:50 method:41")
			method := &QueueDeleteOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		default:
			return nil, fmt.Errorf("Bad method frame, unknown method %d for class %d", mf.MethodId, mf.ClassId)
		}

	case 60: // basic
		switch mf.MethodId {

		case 10: // basic qos
			// fmt.Println("NextMethod: class:60 method:10")
			method := &BasicQos{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 11: // basic qos-ok
			// fmt.Println("NextMethod: class:60 method:11")
			method := &BasicQosOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 20: // basic consume
			// fmt.Println("NextMethod: class:60 method:20")
			method := &BasicConsume{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 21: // basic consume-ok
			// fmt.Println("NextMethod: class:60 method:21")
			method := &BasicConsumeOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 30: // basic cancel
			// fmt.Println("NextMethod: class:60 method:30")
			method := &BasicCancel{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 31: // basic cancel-ok
			// fmt.Println("NextMethod: class:60 method:31")
			method := &BasicCancelOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 40: // basic publish
			// fmt.Println("NextMethod: class:60 method:40")
			method := &BasicPublish{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 50: // basic return
			// fmt.Println("NextMethod: class:60 method:50")
			method := &BasicReturn{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 60: // basic deliver
			// fmt.Println("NextMethod: class:60 method:60")
			method := &BasicDeliver{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 70: // basic get
			// fmt.Println("NextMethod: class:60 method:70")
			method := &BasicGet{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 71: // basic get-ok
			// fmt.Println("NextMethod: class:60 method:71")
			method := &BasicGetOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 72: // basic get-empty
			// fmt.Println("NextMethod: class:60 method:72")
			method := &BasicGetEmpty{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 80: // basic ack
			// fmt.Println("NextMethod: class:60 method:80")
			method := &BasicAck{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 90: // basic reject
			// fmt.Println("NextMethod: class:60 method:90")
			method := &BasicReject{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 100: // basic recover-async
			// fmt.Println("NextMethod: class:60 method:100")
			method := &BasicRecoverAsync{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 110: // basic recover
			// fmt.Println("NextMethod: class:60 method:110")
			method := &BasicRecover{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 111: // basic recover-ok
			// fmt.Println("NextMethod: class:60 method:111")
			method := &BasicRecoverOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 120: // basic nack
			// fmt.Println("NextMethod: class:60 method:120")
			method := &BasicNack{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		default:
			return nil, fmt.Errorf("Bad method frame, unknown method %d for class %d", mf.MethodId, mf.ClassId)
		}

	case 90: // tx
		switch mf.MethodId {

		case 10: // tx select
			// fmt.Println("NextMethod: class:90 method:10")
			method := &TxSelect{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 11: // tx select-ok
			// fmt.Println("NextMethod: class:90 method:11")
			method := &TxSelectOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 20: // tx commit
			// fmt.Println("NextMethod: class:90 method:20")
			method := &TxCommit{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 21: // tx commit-ok
			// fmt.Println("NextMethod: class:90 method:21")
			method := &TxCommitOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 30: // tx rollback
			// fmt.Println("NextMethod: class:90 method:30")
			method := &TxRollback{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 31: // tx rollback-ok
			// fmt.Println("NextMethod: class:90 method:31")
			method := &TxRollbackOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		default:
			return nil, fmt.Errorf("Bad method frame, unknown method %d for class %d", mf.MethodId, mf.ClassId)
		}

	case 85: // confirm
		switch mf.MethodId {

		case 10: // confirm select
			// fmt.Println("NextMethod: class:85 method:10")
			method := &ConfirmSelect{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		case 11: // confirm select-ok
			// fmt.Println("NextMethod: class:85 method:11")
			method := &ConfirmSelectOk{}
			if err = method.Read(r.r); err != nil {
				return
			}
			mf.Method = method

		default:
			return nil, fmt.Errorf("Bad method frame, unknown method %d for class %d", mf.MethodId, mf.ClassId)
		}

	default:
		return nil, fmt.Errorf("Bad method frame, unknown class %d", mf.ClassId)
	}

	return mf, nil
}
