package apiclient

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/fujiwara/trabbits/types"
)

// Use types.ProbeLogEntry instead of local definition
type ProbeLogEntry = types.ProbeLogEntry

// StreamProbeLog streams real-time probe logs from the API via SSE for CLI
func (c *Client) StreamProbeLog(ctx context.Context, proxyID, format string) error {
	if proxyID == "" {
		return fmt.Errorf("proxy ID cannot be empty")
	}

	return c.readProbeLogSSE(ctx, proxyID, func(entry *ProbeLogEntry) error {
		return c.formatProbeLogEntry(entry, format)
	})
}

// StreamProbeLogEntries streams probe logs as structured entries
func (c *Client) StreamProbeLogEntries(ctx context.Context, proxyID string) (<-chan types.ProbeLogEntry, error) {
	logChan := make(chan types.ProbeLogEntry, 100)
	slog.Debug("StreamProbeLogEntries starting", "proxy_id", proxyID)

	go func() {
		defer close(logChan)
		defer slog.Debug("StreamProbeLogEntries finished", "proxy_id", proxyID)

		err := c.readProbeLogSSE(ctx, proxyID, func(entry *types.ProbeLogEntry) error {
			select {
			case logChan <- *entry:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})

		if err != nil && ctx.Err() == nil {
			slog.Error("StreamProbeLogEntries error", "error", err, "proxy_id", proxyID)
		}
	}()

	return logChan, nil
}

// readProbeLogSSE reads SSE stream and calls handler for each probe log entry
func (c *Client) readProbeLogSSE(ctx context.Context, proxyID string, handler func(*types.ProbeLogEntry) error) error {
	fullURL, err := c.buildURL(fmt.Sprintf("clients/%s/probe", proxyID))
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fullURL.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("proxy not found: %s", proxyID)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to stream probe log: %s - %s", resp.Status, string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	var dataBuffer strings.Builder

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			dataBuffer.WriteString(data)
		} else if line == "" {
			// Empty line indicates end of event
			if dataBuffer.Len() > 0 {
				var entry types.ProbeLogEntry
				if err := json.Unmarshal([]byte(dataBuffer.String()), &entry); err != nil {
					slog.Warn("Failed to parse probe log entry", "error", err, "data", dataBuffer.String())
				} else {
					if err := handler(&entry); err != nil {
						return err
					}
				}
				dataBuffer.Reset()
			}
		}
		// Ignore other SSE fields like "event:", "id:", "retry:"
	}

	return scanner.Err()
}

// formatProbeLogEntry formats and displays a probe log entry
func (c *Client) formatProbeLogEntry(entry *types.ProbeLogEntry, format string) error {
	if format == "json" {
		// Re-marshal to JSON for consistent output
		data, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	} else {
		// Default text format
		fmt.Printf("[%s] %s: %s\n",
			entry.Timestamp.Format("15:04:05.000"),
			entry.ProxyID,
			entry.Message)
	}
	return nil
}
