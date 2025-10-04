package trabbits_test

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/fujiwara/trabbits"
	"github.com/fujiwara/trabbits/types"
)

func TestLogBuffer_Add(t *testing.T) {
	buf := trabbits.NewLogBuffer(10)

	// Add a log entry
	entry := types.ProbeLogEntry{
		Timestamp: time.Now(),
		Message:   "test message",
		Attrs: map[string]any{
			"key1": "value1",
			"key2": 42,
		},
	}

	buf.Add(entry)

	// Subscribe and check if we receive the entry
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	ch := buf.Subscribe(ctx, "test-listener")

	select {
	case received := <-ch:
		if received.Message != entry.Message {
			t.Errorf("expected message %q, got %q", entry.Message, received.Message)
		}
		if received.Attrs["key1"] != "value1" {
			t.Errorf("expected key1=value1, got %v", received.Attrs["key1"])
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for log entry")
	}
}

func TestLogBuffer_MaxSize(t *testing.T) {
	maxSize := 5
	buf := trabbits.NewLogBuffer(maxSize)

	// Add more entries than maxSize
	for i := 0; i < 10; i++ {
		entry := types.ProbeLogEntry{
			Timestamp: time.Now(),
			Message:   "message",
			Attrs: map[string]any{
				"index": i,
			},
		}
		buf.Add(entry)
	}

	// Subscribe and check that we only receive the last maxSize entries
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	ch := buf.Subscribe(ctx, "test-listener")

	received := []types.ProbeLogEntry{}
	timeout := time.After(500 * time.Millisecond)

loop:
	for {
		select {
		case entry := <-ch:
			received = append(received, entry)
			if len(received) >= maxSize {
				break loop
			}
		case <-timeout:
			break loop
		}
	}

	if len(received) != maxSize {
		t.Errorf("expected %d entries, got %d", maxSize, len(received))
	}

	// Check that we received the last maxSize entries (indices 5-9)
	if len(received) > 0 {
		firstIndex := received[0].Attrs["index"].(int)
		if firstIndex != 5 {
			t.Errorf("expected first index to be 5, got %d", firstIndex)
		}
	}
}

func TestLogBuffer_MultipleSubscribers(t *testing.T) {
	buf := trabbits.NewLogBuffer(100)

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	// Create multiple subscribers
	ch1 := buf.Subscribe(ctx, "listener-1")
	ch2 := buf.Subscribe(ctx, "listener-2")
	ch3 := buf.Subscribe(ctx, "listener-3")

	// Add a log entry
	entry := types.ProbeLogEntry{
		Timestamp: time.Now(),
		Message:   "broadcast message",
		Attrs:     map[string]any{"test": true},
	}

	buf.Add(entry)

	// All subscribers should receive the entry
	var wg sync.WaitGroup
	wg.Add(3)

	checkReceived := func(ch <-chan types.ProbeLogEntry, name string) {
		defer wg.Done()
		select {
		case received := <-ch:
			if received.Message != entry.Message {
				t.Errorf("%s: expected message %q, got %q", name, entry.Message, received.Message)
			}
		case <-ctx.Done():
			t.Errorf("%s: timeout waiting for log entry", name)
		}
	}

	go checkReceived(ch1, "listener-1")
	go checkReceived(ch2, "listener-2")
	go checkReceived(ch3, "listener-3")

	wg.Wait()
}

func TestLogBuffer_Unsubscribe(t *testing.T) {
	buf := trabbits.NewLogBuffer(10)

	ctx, cancel := context.WithCancel(t.Context())
	ch := buf.Subscribe(ctx, "test-listener")

	// Cancel context to trigger unsubscribe
	cancel()

	// Wait a bit for cleanup
	time.Sleep(100 * time.Millisecond)

	// Channel should be closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed")
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for channel close")
	}
}

func TestServerLogHandler_Handle(t *testing.T) {
	buf := trabbits.NewLogBuffer(100)

	// Create a handler chain with ServerLogHandler
	nextHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	handler := trabbits.NewServerLogHandler(buf, nextHandler)

	logger := slog.New(handler)

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	ch := buf.Subscribe(ctx, "test-listener")

	// Log a message
	logger.Info("test log message", "key1", "value1", "key2", 42)

	// Check if we receive the log entry
	select {
	case entry := <-ch:
		if entry.Message != "test log message" {
			t.Errorf("expected message %q, got %q", "test log message", entry.Message)
		}
		if entry.Attrs["key1"] != "value1" {
			t.Errorf("expected key1=value1, got %v", entry.Attrs["key1"])
		}
		// slog stores integers as int64
		if v, ok := entry.Attrs["key2"].(int64); !ok || v != 42 {
			t.Errorf("expected key2=42, got %v (type %T)", entry.Attrs["key2"], entry.Attrs["key2"])
		}
		if entry.Attrs["level"] != "INFO" {
			t.Errorf("expected level=INFO, got %v", entry.Attrs["level"])
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for log entry")
	}
}

func TestServerLogHandler_WithAttrs(t *testing.T) {
	buf := trabbits.NewLogBuffer(100)

	nextHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	handler := trabbits.NewServerLogHandler(buf, nextHandler)

	// Create logger with pre-configured attributes
	logger := slog.New(handler).With("component", "test", "version", "1.0")

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	ch := buf.Subscribe(ctx, "test-listener")

	// Log a message
	logger.Info("test message", "extra", "data")

	// Check if we receive the log entry with all attributes
	select {
	case entry := <-ch:
		if entry.Message != "test message" {
			t.Errorf("expected message %q, got %q", "test message", entry.Message)
		}
		if entry.Attrs["component"] != "test" {
			t.Errorf("expected component=test, got %v", entry.Attrs["component"])
		}
		if entry.Attrs["version"] != "1.0" {
			t.Errorf("expected version=1.0, got %v", entry.Attrs["version"])
		}
		if entry.Attrs["extra"] != "data" {
			t.Errorf("expected extra=data, got %v", entry.Attrs["extra"])
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for log entry")
	}
}

func TestServerLogHandler_WithGroup(t *testing.T) {
	buf := trabbits.NewLogBuffer(100)

	nextHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	handler := trabbits.NewServerLogHandler(buf, nextHandler)

	logger := slog.New(handler).WithGroup("request")

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	ch := buf.Subscribe(ctx, "test-listener")

	// Log a message with grouped attributes
	logger.Info("test message", "method", "GET", "path", "/api")

	// Check if we receive the log entry
	select {
	case entry := <-ch:
		if entry.Message != "test message" {
			t.Errorf("expected message %q, got %q", "test message", entry.Message)
		}
		// Grouped attributes should be present
		// The exact structure depends on slog implementation
		t.Logf("Received attributes: %+v", entry.Attrs)
	case <-ctx.Done():
		t.Fatal("timeout waiting for log entry")
	}
}

func TestLogBuffer_ConcurrentAddAndSubscribe(t *testing.T) {
	buf := trabbits.NewLogBuffer(1000)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	// Create multiple subscribers
	numSubscribers := 5
	subscribers := make([]<-chan types.ProbeLogEntry, numSubscribers)
	for i := 0; i < numSubscribers; i++ {
		subscribers[i] = buf.Subscribe(ctx, "listener-"+string(rune('A'+i)))
	}

	// Concurrently add log entries
	numEntries := 100
	var wg sync.WaitGroup

	// Add entries concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numEntries; i++ {
			entry := types.ProbeLogEntry{
				Timestamp: time.Now(),
				Message:   "concurrent message",
				Attrs: map[string]any{
					"index": i,
				},
			}
			buf.Add(entry)
			time.Sleep(time.Millisecond) // Small delay to simulate real usage
		}
	}()

	// Count received entries for each subscriber
	for idx, ch := range subscribers {
		wg.Add(1)
		go func(listenerIdx int, ch <-chan types.ProbeLogEntry) {
			defer wg.Done()
			count := 0
			timeout := time.After(2 * time.Second)
			for {
				select {
				case _, ok := <-ch:
					if !ok {
						return
					}
					count++
					if count >= numEntries {
						return
					}
				case <-timeout:
					t.Logf("listener-%d received %d entries", listenerIdx, count)
					return
				case <-ctx.Done():
					return
				}
			}
		}(idx, ch)
	}

	wg.Wait()
}
