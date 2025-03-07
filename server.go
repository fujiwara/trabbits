// Copyright (c) 2021 VMware, Inc. or its affiliates. All Rights Reserved.
// Copyright (c) 2012-2021, Sean Treadway, SoundCloud Ltd.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package trabbits

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"reflect"
)

func RunServer() {
	listener, err := net.Listen("tcp", ":5672")
	if err != nil {
		fmt.Println("Failed to start AMQP server:", err)
		return
	}
	defer listener.Close()

	fmt.Println("AMQP Server started on port 5672...")

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Failed to accept connection:", err)
			continue
		}
		go handleConnection(conn)
	}
}

type Client struct {
	conn    net.Conn
	channel map[uint16]bool
}

var amqpHeader = []byte{'A', 'M', 'Q', 'P', 0, 0, 9, 1}

func handleConnection(conn net.Conn) {
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
	start := &connectionStart{
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
	msg := connectionStartOk{}
	_, err := s.recv(0, &msg)
	if err != nil {
		fmt.Println("Failed to read Connection.Start-Ok:", err)
		return
	}
	log.Printf("Connection.Start-Ok: %#v", msg)
	// TODO authentificate

	// Connection.Tune 送信
	tune := &connectionTune{
		ChannelMax: 1023,
		FrameMax:   131072, // 128KB
		Heartbeat:  60,
	}
	if err := s.send(0, tune); err != nil {
		fmt.Println("Failed to write Connection.Tune:", err)
		return
	}

	// Connection.Tune-Ok 受信
	okmsg := connectionTuneOk{}
	_, err = s.recv(0, &okmsg)
	if err != nil {
		fmt.Println("Failed to read Connection.Tune-Ok:", err)
		return
	}
	log.Printf("Connection.Tune-Ok: %#v", okmsg)

	// Connection.Open 受信
	openmsg := connectionOpen{}
	_, err = s.recv(0, &openmsg)
	if err != nil {
		fmt.Println("Failed to read Connection.Open:", err)
		return
	}
	log.Printf("Connection.Open: %#v", openmsg)

	// Connection.Open-Ok 送信
	openOk := &connectionOpenOk{}
	if err := s.send(0, openOk); err != nil {
		fmt.Println("Failed to write Connection.Open-Ok:", err)
		return
	}
	log.Printf("Connection opened: %#v", openOk)
}

type Server struct {
	r reader             // framer <- client
	w writer             // framer -> client
	S io.ReadWriteCloser // Server IO
	C io.ReadWriteCloser // Client IO
}

func NewServer(serverIO io.ReadWriteCloser) *Server {
	return &Server{
		r: reader{serverIO},
		w: writer{serverIO},
	}
}

func (t *Server) send(channel int, m message) error {
	if msg, ok := m.(messageWithContent); ok {
		props, body := msg.getContent()
		class, _ := msg.id()
		if err := t.w.WriteFrame(&methodFrame{
			ChannelId: uint16(channel),
			Method:    msg,
		}); err != nil {
			return fmt.Errorf("WriteFrame error: %w", err)
		}
		if err := t.w.WriteFrame(&headerFrame{
			ChannelId:  uint16(channel),
			ClassId:    class,
			Size:       uint64(len(body)),
			Properties: props,
		}); err != nil {
			return fmt.Errorf("WriteFrame error: %w", err)
		}
		if err := t.w.WriteFrame(&bodyFrame{
			ChannelId: uint16(channel),
			Body:      body,
		}); err != nil {
			return fmt.Errorf("WriteFrame error: %w", err)
		}
	} else {
		if err := t.w.WriteFrame(&methodFrame{
			ChannelId: uint16(channel),
			Method:    m,
		}); err != nil {
			return fmt.Errorf("WriteFrame error: %w", err)
		}
	}
	return nil
}

func (t *Server) recv(channel int, m message) (message, error) {
	var remaining int
	var header *headerFrame
	var body []byte

	for {
		frame, err := t.r.ReadFrame()
		if err != nil {
			return nil, fmt.Errorf("frame err, read: %w", err)
		}

		if frame.channel() != uint16(channel) {
			return nil, fmt.Errorf("expected frame on channel %d, got channel %d", channel, frame.channel())
		}

		switch f := frame.(type) {
		case *heartbeatFrame:
			// drop

		case *headerFrame:
			// start content state
			header = f
			remaining = int(header.Size)
			if remaining == 0 {
				m.(messageWithContent).setContent(header.Properties, nil)
				return m, nil
			}

		case *bodyFrame:
			// continue until terminated
			body = append(body, f.Body...)
			remaining -= len(f.Body)
			if remaining <= 0 {
				m.(messageWithContent).setContent(header.Properties, body)
				return m, nil
			}

		case *methodFrame:
			if reflect.TypeOf(m) == reflect.TypeOf(f.Method) {
				wantv := reflect.ValueOf(m).Elem()
				havev := reflect.ValueOf(f.Method).Elem()
				wantv.Set(havev)
				if _, ok := m.(messageWithContent); !ok {
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
