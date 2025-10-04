package tui

import (
	"context"
	"log/slog"
)

// TUIHandler is a custom slog.Handler that sends logs to the TUI
type TUIHandler struct {
	logChan chan<- LogEntry
	next    slog.Handler
}

// NewTUIHandler creates a new TUI handler that sends logs to the given channel
// and also forwards to the next handler (for console output)
func NewTUIHandler(logChan chan<- LogEntry, next slog.Handler) *TUIHandler {
	return &TUIHandler{
		logChan: logChan,
		next:    next,
	}
}

// Enabled reports whether the handler handles records at the given level
func (h *TUIHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

// Handle handles the Record
func (h *TUIHandler) Handle(ctx context.Context, r slog.Record) error {
	// Extract attributes
	attrs := make(map[string]any)
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})

	// Send to TUI channel (non-blocking)
	select {
	case h.logChan <- LogEntry{
		Time:    r.Time,
		Level:   r.Level.String(),
		Message: r.Message,
		Attrs:   attrs,
	}:
	default:
		// Channel full, skip this log to avoid blocking
	}

	// Forward to next handler
	return h.next.Handle(ctx, r)
}

// WithAttrs returns a new Handler whose attributes consist of
// both the receiver's attributes and the arguments
func (h *TUIHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &TUIHandler{
		logChan: h.logChan,
		next:    h.next.WithAttrs(attrs),
	}
}

// WithGroup returns a new Handler with the given group appended to
// the receiver's existing groups
func (h *TUIHandler) WithGroup(name string) slog.Handler {
	return &TUIHandler{
		logChan: h.logChan,
		next:    h.next.WithGroup(name),
	}
}
