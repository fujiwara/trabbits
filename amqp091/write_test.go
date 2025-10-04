package amqp091_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/fujiwara/trabbits/amqp091"
)

// TestMethodFrameWrite tests that MethodFrame.Write produces correct output
func TestMethodFrameWrite(t *testing.T) {
	// Create a simple MethodFrame with ConnectionStart
	frame := &amqp091.MethodFrame{
		ChannelId: 0,
		Method: &amqp091.ConnectionStart{
			VersionMajor: 0,
			VersionMinor: 9,
			ServerProperties: amqp091.Table{
				"product": "Test",
			},
			Mechanisms: "PLAIN",
			Locales:    "en_US",
		},
	}

	var buf bytes.Buffer
	if err := frame.Write(&buf); err != nil {
		t.Fatalf("MethodFrame.Write failed: %v", err)
	}

	// Verify frame structure
	data := buf.Bytes()
	if len(data) < 8 {
		t.Fatalf("Frame too short: %d bytes", len(data))
	}

	// Check frame type (first byte should be frameMethod = 1)
	if data[0] != 1 {
		t.Errorf("Expected frame type 1 (method), got %d", data[0])
	}

	// Check channel ID (bytes 1-2)
	channel := uint16(data[1])<<8 | uint16(data[2])
	if channel != 0 {
		t.Errorf("Expected channel 0, got %d", channel)
	}

	// Check frame end marker (last byte should be 0xCE)
	if data[len(data)-1] != 0xCE {
		t.Errorf("Expected frame end marker 0xCE, got 0x%X", data[len(data)-1])
	}
}

// TestHeaderFrameWrite tests that HeaderFrame.Write produces correct output
func TestHeaderFrameWrite(t *testing.T) {
	frame := &amqp091.HeaderFrame{
		ChannelId: 1,
		ClassId:   60, // Basic class
		Size:      100,
		Properties: amqp091.Properties{
			ContentType:  "text/plain",
			DeliveryMode: 2,
		},
	}

	var buf bytes.Buffer
	if err := frame.Write(&buf); err != nil {
		t.Fatalf("HeaderFrame.Write failed: %v", err)
	}

	data := buf.Bytes()
	if len(data) < 8 {
		t.Fatalf("Frame too short: %d bytes", len(data))
	}

	// Check frame type (should be frameHeader = 2)
	if data[0] != 2 {
		t.Errorf("Expected frame type 2 (header), got %d", data[0])
	}

	// Check channel ID
	channel := uint16(data[1])<<8 | uint16(data[2])
	if channel != 1 {
		t.Errorf("Expected channel 1, got %d", channel)
	}

	// Check frame end marker
	if data[len(data)-1] != 0xCE {
		t.Errorf("Expected frame end marker 0xCE, got 0x%X", data[len(data)-1])
	}
}

// TestBodyFrameWrite tests that BodyFrame.Write produces correct output
func TestBodyFrameWrite(t *testing.T) {
	body := []byte("Hello, World!")
	frame := &amqp091.BodyFrame{
		ChannelId: 1,
		Body:      body,
	}

	var buf bytes.Buffer
	if err := frame.Write(&buf); err != nil {
		t.Fatalf("BodyFrame.Write failed: %v", err)
	}

	data := buf.Bytes()
	if len(data) < 8+len(body) {
		t.Fatalf("Frame too short: %d bytes, expected at least %d", len(data), 8+len(body))
	}

	// Check frame type (should be frameBody = 3)
	if data[0] != 3 {
		t.Errorf("Expected frame type 3 (body), got %d", data[0])
	}

	// Check channel ID
	channel := uint16(data[1])<<8 | uint16(data[2])
	if channel != 1 {
		t.Errorf("Expected channel 1, got %d", channel)
	}

	// Check payload size
	size := uint32(data[3])<<24 | uint32(data[4])<<16 | uint32(data[5])<<8 | uint32(data[6])
	if size != uint32(len(body)) {
		t.Errorf("Expected size %d, got %d", len(body), size)
	}

	// Check body content
	actualBody := data[7 : 7+len(body)]
	if !bytes.Equal(actualBody, body) {
		t.Errorf("Body mismatch: expected %q, got %q", body, actualBody)
	}

	// Check frame end marker
	if data[len(data)-1] != 0xCE {
		t.Errorf("Expected frame end marker 0xCE, got 0x%X", data[len(data)-1])
	}
}

// TestHeartbeatFrameWrite tests that HeartbeatFrame.Write produces correct output
func TestHeartbeatFrameWrite(t *testing.T) {
	frame := &amqp091.HeartbeatFrame{
		ChannelId: 0,
	}

	var buf bytes.Buffer
	if err := frame.Write(&buf); err != nil {
		t.Fatalf("HeartbeatFrame.Write failed: %v", err)
	}

	data := buf.Bytes()

	// Heartbeat frame should be exactly 8 bytes (7 header + 0 payload + 1 end)
	if len(data) != 8 {
		t.Fatalf("Expected heartbeat frame to be 8 bytes, got %d", len(data))
	}

	// Check frame type (should be frameHeartbeat = 8)
	if data[0] != 8 {
		t.Errorf("Expected frame type 8 (heartbeat), got %d", data[0])
	}

	// Check payload size (should be 0)
	size := uint32(data[3])<<24 | uint32(data[4])<<16 | uint32(data[5])<<8 | uint32(data[6])
	if size != 0 {
		t.Errorf("Expected size 0, got %d", size)
	}

	// Check frame end marker
	if data[len(data)-1] != 0xCE {
		t.Errorf("Expected frame end marker 0xCE, got 0x%X", data[len(data)-1])
	}
}

// TestLargeBodyFrame tests that large body frames are written correctly
func TestLargeBodyFrame(t *testing.T) {
	// Create a large body (10KB)
	body := make([]byte, 10*1024)
	for i := range body {
		body[i] = byte(i % 256)
	}

	frame := &amqp091.BodyFrame{
		ChannelId: 1,
		Body:      body,
	}

	var buf bytes.Buffer
	if err := frame.Write(&buf); err != nil {
		t.Fatalf("BodyFrame.Write failed: %v", err)
	}

	data := buf.Bytes()

	// Check frame end marker
	if data[len(data)-1] != 0xCE {
		t.Errorf("Expected frame end marker 0xCE, got 0x%X", data[len(data)-1])
	}

	// Check body content
	actualBody := data[7 : 7+len(body)]
	if !bytes.Equal(actualBody, body) {
		t.Errorf("Large body content mismatch")
	}
}

// writerFunc adapts a function to io.Writer interface for testing
type writerFunc func(p []byte) (n int, err error)

func (f writerFunc) Write(p []byte) (n int, err error) {
	return f(p)
}

// TestWriteError tests error handling in frame writing
func TestWriteError(t *testing.T) {
	frame := &amqp091.BodyFrame{
		ChannelId: 1,
		Body:      []byte("test"),
	}

	// Create a writer that always returns an error
	errorWriter := writerFunc(func(p []byte) (int, error) {
		return 0, io.ErrShortWrite
	})

	err := frame.Write(errorWriter)
	if err == nil {
		t.Error("Expected error when writer fails, got nil")
	}
}
