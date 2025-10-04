// MIT License
// Copyright (c) 2025 FUJIWARA Shunichiro

package trabbits_test

import (
	"fmt"
	"net"
	"testing"
	"time"
)

func TestHandshakeTimeout(t *testing.T) {
	addr := fmt.Sprintf("localhost:%d", testProxyPort)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to connect to test server: %v", err)
	}
	defer conn.Close()

	time.Sleep(1500 * time.Millisecond) // Wait longer than the handshake timeout
	_, err = conn.Read(make([]byte, 1))
	if err == nil {
		t.Fatal("Expected connection to be closed by server, but read succeeded")
	}
	t.Logf("Connection closed as expected: %v", err)
}

func TestHandshakeTimeoutAfterAMQPHeader(t *testing.T) {
	addr := fmt.Sprintf("localhost:%d", testProxyPort)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to connect to test server: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("AMQP\x00\x00\x09\x01")); err != nil {
		t.Fatalf("Failed to send AMQP header: %v", err)
	}
	buf := make([]byte, 1024) // ensure buffer is large enough for amqp header response
	if n, err := conn.Read(buf); err != nil {
		t.Fatalf("Failed to read from server: %v", err)
	} else {
		t.Logf("Received %d bytes from server after sending AMQP header", n)
	}

	time.Sleep(1500 * time.Millisecond) // Wait longer than the handshake timeout
	_, err = conn.Read(buf)
	if err == nil {
		t.Fatal("Expected connection to be closed by server, but Read succeeded")
	}
	t.Logf("Connection closed as expected: %v", err)
}
