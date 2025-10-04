package trabbits

import (
	"context"
	"log/slog"
	"sync"

	"github.com/fujiwara/trabbits/types"
)

// LogBuffer stores recent log entries and broadcasts them to listeners
type LogBuffer struct {
	mu        sync.RWMutex
	entries   []types.ProbeLogEntry // Reuse ProbeLogEntry type for consistency
	maxSize   int
	listeners map[string]chan types.ProbeLogEntry
}

// NewLogBuffer creates a new log buffer with the specified maximum size
func NewLogBuffer(maxSize int) *LogBuffer {
	return &LogBuffer{
		entries:   make([]types.ProbeLogEntry, 0, maxSize),
		maxSize:   maxSize,
		listeners: make(map[string]chan types.ProbeLogEntry),
	}
}

// Add adds a log entry to the buffer and broadcasts to all listeners
func (b *LogBuffer) Add(entry types.ProbeLogEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Add to buffer (keep last maxSize entries)
	b.entries = append(b.entries, entry)
	if len(b.entries) > b.maxSize {
		b.entries = b.entries[len(b.entries)-b.maxSize:]
	}

	// Broadcast to all listeners (non-blocking)
	for _, ch := range b.listeners {
		select {
		case ch <- entry:
		default:
			// Listener channel full, skip
		}
	}
}

// Subscribe creates a new listener channel
func (b *LogBuffer) Subscribe(ctx context.Context, listenerID string) <-chan types.ProbeLogEntry {
	ch := make(chan types.ProbeLogEntry, 100)

	b.mu.Lock()
	b.listeners[listenerID] = ch
	// Send recent entries to new subscriber
	recentEntries := make([]types.ProbeLogEntry, len(b.entries))
	copy(recentEntries, b.entries)
	b.mu.Unlock()

	// Send recent entries in a goroutine to avoid blocking
	go func() {
		for _, entry := range recentEntries {
			select {
			case ch <- entry:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Clean up on context cancellation
	go func() {
		<-ctx.Done()
		b.Unsubscribe(listenerID)
	}()

	return ch
}

// Unsubscribe removes a listener
func (b *LogBuffer) Unsubscribe(listenerID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if ch, ok := b.listeners[listenerID]; ok {
		close(ch)
		delete(b.listeners, listenerID)
	}
}

// ServerLogHandler is a custom slog.Handler that sends logs to the log buffer
type ServerLogHandler struct {
	logBuffer *LogBuffer
	next      slog.Handler
	attrs     []slog.Attr // Accumulated attributes from WithAttrs
	groups    []string    // Accumulated group names from WithGroup
}

// NewServerLogHandler creates a new handler that sends logs to the buffer
func NewServerLogHandler(logBuffer *LogBuffer, next slog.Handler) *ServerLogHandler {
	return &ServerLogHandler{
		logBuffer: logBuffer,
		next:      next,
		attrs:     []slog.Attr{},
		groups:    []string{},
	}
}

// Enabled reports whether the handler handles records at the given level
func (h *ServerLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

// Handle handles the Record
func (h *ServerLogHandler) Handle(ctx context.Context, r slog.Record) error {
	// Extract attributes including those from WithAttrs
	attrs := make(map[string]any)

	// First add accumulated attrs from WithAttrs
	for _, attr := range h.attrs {
		attrs[attr.Key] = attr.Value.Any()
	}

	// Then add attrs from the record
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})

	// Add log level to attrs
	attrs["level"] = r.Level.String()

	// Add to buffer
	h.logBuffer.Add(types.ProbeLogEntry{
		Timestamp: r.Time,
		Message:   r.Message,
		Attrs:     attrs,
	})

	// Forward to next handler
	return h.next.Handle(ctx, r)
}

// WithAttrs returns a new Handler whose attributes consist of
// both the receiver's attributes and the arguments
func (h *ServerLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// Accumulate attributes
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)

	return &ServerLogHandler{
		logBuffer: h.logBuffer,
		next:      h.next.WithAttrs(attrs),
		attrs:     newAttrs,
		groups:    h.groups,
	}
}

// WithGroup returns a new Handler with the given group appended to
// the receiver's existing groups
func (h *ServerLogHandler) WithGroup(name string) slog.Handler {
	// Accumulate group names
	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name

	return &ServerLogHandler{
		logBuffer: h.logBuffer,
		next:      h.next.WithGroup(name),
		attrs:     h.attrs,
		groups:    newGroups,
	}
}
