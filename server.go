// Copyright (c) 2021 VMware, Inc. or its affiliates. All Rights Reserved.
// Copyright (c) 2012-2021, Sean Treadway, SoundCloud Ltd.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package trabbits

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"reflect"

	"github.com/fujiwara/trabbits/amqp091"
)

func Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", ":5672")
	if err != nil {
		return fmt.Errorf("failed to start AMQP server: %w", err)
	}
	defer listener.Close()

	log.Println("AMQP Server started on port 5672...")

	go func() {
		<-ctx.Done()
		if err := listener.Close(); err != nil {
			log.Println("Failed to close listener:", err)
		}
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				log.Println("AMQP server stopped")
				return nil
			default:
			}
			log.Println("Failed to accept connection:", err)
			continue
		}
		go handleConnection(ctx, conn)
	}
}

type Client struct {
	conn    net.Conn
	channel map[uint16]bool
}

var amqpHeader = []byte{'A', 'M', 'Q', 'P', 0, 0, 9, 1}

func handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	_ = &Client{
		conn:    conn,
		channel: make(map[uint16]bool),
	}

	// AMQP プロトコルヘッダー受信
	header := make([]byte, 8)
	if _, err := io.ReadFull(conn, header); err != nil {
		fmt.Println("Failed to read AMQP header:", err)
		return
	}
	if !bytes.Equal(header, amqpHeader) {
		fmt.Println("Invalid AMQP protocol header")
		return
	}
	log.Printf("connected from %s", conn.RemoteAddr())

	s := NewServer(conn)
	// Connection.Start 送信
	start := &amqp091.ConnectionStart{
		VersionMajor: 0,
		VersionMinor: 9,
		Mechanisms:   "PLAIN",
		Locales:      "en_US",
	}
	if err := s.send(0, start); err != nil {
		fmt.Println("Failed to write Connection.Start:", err)
		return
	}

	// Connection.Start-Ok 受信（認証情報含む）
	msg := amqp091.ConnectionStartOk{}
	_, err := s.recv(0, &msg)
	if err != nil {
		fmt.Println("Failed to read Connection.Start-Ok:", err)
		return
	}
	log.Printf("Connection.Start-Ok: %#v", msg)
	// TODO authentificate

	// Connection.Tune 送信
	tune := &amqp091.ConnectionTune{
		ChannelMax: 1023,
		FrameMax:   131072, // 128KB
		Heartbeat:  60,
	}
	if err := s.send(0, tune); err != nil {
		fmt.Println("Failed to write Connection.Tune:", err)
		return
	}

	// Connection.Tune-Ok 受信
	okmsg := amqp091.ConnectionTuneOk{}
	_, err = s.recv(0, &okmsg)
	if err != nil {
		fmt.Println("Failed to read Connection.Tune-Ok:", err)
		return
	}
	log.Printf("Connection.Tune-Ok: %#v", okmsg)

	// Connection.Open 受信
	openmsg := amqp091.ConnectionOpen{}
	_, err = s.recv(0, &openmsg)
	if err != nil {
		fmt.Println("Failed to read Connection.Open:", err)
		return
	}
	log.Printf("Connection.Open: %#v", openmsg)

	// Connection.Open-Ok 送信
	openOk := &amqp091.ConnectionOpenOk{}
	if err := s.send(0, openOk); err != nil {
		fmt.Println("Failed to write Connection.Open-Ok:", err)
		return
	}
	log.Printf("Connection opened: %#v", openOk)
}

type Server struct {
	r *amqp091.Reader // framer <- client
	w *amqp091.Writer // framer -> client
}

func NewServer(serverIO io.ReadWriteCloser) *Server {
	return &Server{
		r: amqp091.NewReader(serverIO),
		w: amqp091.NewWriter(serverIO),
	}
}

func (t *Server) send(channel int, m amqp091.Message) error {
	if msg, ok := m.(amqp091.MessageWithContent); ok {
		props, body := msg.GetContent()
		class, _ := msg.ID()
		if err := t.w.WriteFrame(&amqp091.MethodFrame{
			ChannelId: uint16(channel),
			Method:    msg,
		}); err != nil {
			return fmt.Errorf("WriteFrame error: %w", err)
		}
		if err := t.w.WriteFrame(&amqp091.HeaderFrame{
			ChannelId:  uint16(channel),
			ClassId:    class,
			Size:       uint64(len(body)),
			Properties: props,
		}); err != nil {
			return fmt.Errorf("WriteFrame error: %w", err)
		}
		if err := t.w.WriteFrame(&amqp091.BodyFrame{
			ChannelId: uint16(channel),
			Body:      body,
		}); err != nil {
			return fmt.Errorf("WriteFrame error: %w", err)
		}
	} else {
		if err := t.w.WriteFrame(&amqp091.MethodFrame{
			ChannelId: uint16(channel),
			Method:    m,
		}); err != nil {
			return fmt.Errorf("WriteFrame error: %w", err)
		}
	}
	return nil
}

func (t *Server) recv(channel int, m amqp091.Message) (amqp091.Message, error) {
	var remaining int
	var header *amqp091.HeaderFrame
	var body []byte

	for {
		frame, err := t.r.ReadFrame()
		if err != nil {
			return nil, fmt.Errorf("frame err, read: %w", err)
		}

		if frame.Channel() != uint16(channel) {
			return nil, fmt.Errorf("expected frame on channel %d, got channel %d", channel, frame.Channel())
		}

		switch f := frame.(type) {
		case *amqp091.HeartbeatFrame:
			// drop

		case *amqp091.HeaderFrame:
			// start content state
			header = f
			remaining = int(header.Size)
			if remaining == 0 {
				m.(amqp091.MessageWithContent).SetContent(header.Properties, nil)
				return m, nil
			}

		case *amqp091.BodyFrame:
			// continue until terminated
			body = append(body, f.Body...)
			remaining -= len(f.Body)
			if remaining <= 0 {
				m.(amqp091.MessageWithContent).SetContent(header.Properties, body)
				return m, nil
			}

		case *amqp091.MethodFrame:
			if reflect.TypeOf(m) == reflect.TypeOf(f.Method) {
				wantv := reflect.ValueOf(m).Elem()
				havev := reflect.ValueOf(f.Method).Elem()
				wantv.Set(havev)
				if _, ok := m.(amqp091.MessageWithContent); !ok {
					return m, nil
				}
			} else {
				return nil, fmt.Errorf("expected method type: %T, got: %T", m, f.Method)
			}

		default:
			return nil, fmt.Errorf("unexpected frame: %+v", f)
		}
	}
}
