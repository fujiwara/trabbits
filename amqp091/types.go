// Copyright (c) 2021 VMware, Inc. or its affiliates. All Rights Reserved.
// Copyright (c) 2012-2021, Sean Treadway, SoundCloud Ltd.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package amqp091

import (
	"fmt"
	"io"
	"time"
)

// The property flags are an array of bits that indicate the presence or
// absence of each property value in sequence.  The bits are ordered from most
// high to low - bit 15 indicates the first property.
const (
	flagContentType     = 0x8000
	flagContentEncoding = 0x4000
	flagHeaders         = 0x2000
	flagDeliveryMode    = 0x1000
	flagPriority        = 0x0800
	flagCorrelationId   = 0x0400
	flagReplyTo         = 0x0200
	flagExpiration      = 0x0100
	flagMessageId       = 0x0080
	flagTimestamp       = 0x0040
	flagType            = 0x0020
	flagUserId          = 0x0010
	flagAppId           = 0x0008
	flagReserved1       = 0x0004
)

type Table map[string]interface{}

// Used by header frames to capture routing and header information
type Properties struct {
	ContentType     string    // MIME content type
	ContentEncoding string    // MIME content encoding
	Headers         Table     // Application or header exchange table
	DeliveryMode    uint8     // queue implementation use - Transient (1) or Persistent (2)
	Priority        uint8     // queue implementation use - 0 to 9
	CorrelationId   string    // application use - correlation identifier
	ReplyTo         string    // application use - address to to reply to (ex: RPC)
	Expiration      string    // implementation use - message expiration spec
	MessageId       string    // application use - message identifier
	Timestamp       time.Time // application use - message timestamp
	Type            string    // application use - message type name
	UserId          string    // application use - creating user id
	AppId           string    // application use - creating application
	reserved1       string    // was cluster-id - process for buffer consumption
}

type Reader struct {
	r io.Reader
}

type Writer struct {
	w io.Writer
}

func NewReader(r io.Reader) *Reader {
	return &Reader{r: r}
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

type Frame interface {
	Write(io.Writer) error
	Channel() uint16
}

var (
	// ErrFrame is returned when the protocol frame cannot be read from the
	// server, indicating an unsupported protocol or unsupported frame type.
	ErrFrame = &Error{Code: FrameError, Reason: "frame could not be parsed"}

	// ErrSyntax is hard protocol error, indicating an unsupported protocol,
	// implementation or encoding.
	ErrSyntax = &Error{Code: SyntaxError, Reason: "invalid field or value inside of a frame"}

	// ErrFieldType is returned when writing a message containing a Go type unsupported by AMQP.
	ErrFieldType = &Error{Code: SyntaxError, Reason: "unsupported table field type"}
)

// Error captures the code and reason a channel or connection has been closed
// by the server.
type Error struct {
	Code    int    // constant code from the specification
	Reason  string // description of the error
	Server  bool   // true when initiated from the server, false when from this library
	Recover bool   // true when this error can be recovered by retrying later or with different parameters
}

func newError(code uint16, text string) *Error {
	return &Error{
		Code:    int(code),
		Reason:  text,
		Recover: isSoftExceptionCode(int(code)),
		Server:  true,
	}
}

func (e *Error) Error() string {
	return fmt.Sprintf("Exception (%d) Reason: %q", e.Code, e.Reason)
}

// Recoverable returns true if the error can be recovered by retrying later or with different parameters.
// Returns the value of the Recover field.
func (e *Error) Recoverable() bool {
	return e.Recover
}

// Decimal matches the AMQP decimal type.  Scale is the number of decimal
// digits Scale == 2, Value == 12345, Decimal == 123.45
type Decimal struct {
	Scale uint8
	Value int32
}

/*
Method frames carry the high-level protocol commands (which we call "methods").
One method frame carries one command.  The method frame payload has this format:

	0          2           4
	+----------+-----------+-------------- - -
	| class-id | method-id | arguments...
	+----------+-----------+-------------- - -
	   short      short    ...

To process a method frame, we:
 1. Read the method frame payload.
 2. Unpack it into a structure.  A given method always has the same structure,
    so we can unpack the method rapidly.  3. Check that the method is allowed in
    the current context.
 4. Check that the method arguments are valid.
 5. Execute the method.

Method frame bodies are constructed as a list of AMQP data fields (bits,
integers, strings and string tables).  The marshalling code is trivially
generated directly from the protocol specifications, and can be very rapid.
*/
type MethodFrame struct {
	ChannelId uint16
	ClassId   uint16
	MethodId  uint16
	Method    Message
}

func (f *MethodFrame) Channel() uint16 { return f.ChannelId }

type Message interface {
	ID() (uint16, uint16)
	Wait() bool
	Read(io.Reader) error
	Write(io.Writer) error
}

type MessageWithContent interface {
	Message
	GetContent() (Properties, []byte)
	SetContent(Properties, []byte)
}

/*
Heartbeating is a technique designed to undo one of TCP/IP's features, namely
its ability to recover from a broken physical connection by closing only after
a quite long time-out.  In some scenarios we need to know very rapidly if a
peer is disconnected or not responding for other reasons (e.g. it is looping).
Since heartbeating can be done at a low level, we implement this as a special
type of frame that peers exchange at the transport level, rather than as a
class method.
*/
type HeartbeatFrame struct {
	ChannelId uint16
}

func (f *HeartbeatFrame) Channel() uint16 { return f.ChannelId }

/*
Certain methods (such as Basic.Publish, Basic.Deliver, etc.) are formally
defined as carrying content.  When a peer sends such a method frame, it always
follows it with a content header and zero or more content body frames.

A content header frame has this format:

	0          2        4           12               14
	+----------+--------+-----------+----------------+------------- - -
	| class-id | weight | body size | property flags | property list...
	+----------+--------+-----------+----------------+------------- - -
	  short     short    long long       short        remainder...

We place content body in distinct frames (rather than including it in the
method) so that AMQP may support "zero copy" techniques in which content is
never marshalled or encoded.  We place the content properties in their own
frame so that recipients can selectively discard contents they do not want to
process
*/
type HeaderFrame struct {
	ChannelId  uint16
	ClassId    uint16
	weight     uint16
	Size       uint64
	Properties Properties
}

func (f *HeaderFrame) Channel() uint16 { return f.ChannelId }

/*
Content is the application data we carry from client-to-client via the AMQP
server.  Content is, roughly speaking, a set of properties plus a binary data
part.  The set of allowed properties are defined by the Basic class, and these
form the "content header frame".  The data can be any size, and MAY be broken
into several (or many) chunks, each forming a "content body frame".

Looking at the frames for a specific channel, as they pass on the wire, we
might see something like this:

	[method]
	[method] [header] [body] [body]
	[method]
	...
*/
type BodyFrame struct {
	ChannelId uint16
	Body      []byte
}

func (f *BodyFrame) Channel() uint16 { return f.ChannelId }
