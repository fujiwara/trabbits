package trabbits

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/fujiwara/trabbits/config"
	"github.com/fujiwara/trabbits/tui"
	"github.com/fujiwara/trabbits/types"
)

func manageConfig(ctx context.Context, opt *CLI) error {
	switch opt.Manage.Config.Command {
	case "get":
		return manageConfigGet(ctx, opt)
	case "diff":
		return manageConfigDiff(ctx, opt)
	case "put":
		return manageConfigPut(ctx, opt)
	case "reload":
		return manageConfigReload(ctx, opt)
	default:
		return fmt.Errorf("unknown command: %s", opt.Manage.Config.Command)
	}
}

func manageConfigGet(ctx context.Context, opt *CLI) error {
	client := NewAPIClient(opt.APISocket)
	cfg, err := client.getConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}
	fmt.Print(cfg.String())
	return nil
}

func manageConfigDiff(ctx context.Context, opt *CLI) error {
	if opt.Manage.Config.File == "" {
		return fmt.Errorf("configuration file is required for diff command")
	}

	client := NewAPIClient(opt.APISocket)
	diff, err := client.diffConfigFromFile(ctx, opt.Manage.Config.File)
	if err != nil {
		return fmt.Errorf("failed to get diff: %w", err)
	}

	if diff != "" {
		fmt.Print(ColoredDiff(diff))
	}

	return nil
}

func manageConfigPut(ctx context.Context, opt *CLI) error {
	if opt.Manage.Config.File == "" {
		return fmt.Errorf("configuration file is required for put command")
	}

	client := NewAPIClient(opt.APISocket)
	return client.putConfigFromFile(ctx, opt.Manage.Config.File)
}

func manageConfigReload(ctx context.Context, opt *CLI) error {
	client := NewAPIClient(opt.APISocket)
	cfg, err := client.reloadConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to reload config: %w", err)
	}
	slog.Info("Configuration reloaded successfully")
	fmt.Print(cfg.String())
	return nil
}

// ColoredDiff adds color to diff output (+/- lines)
func ColoredDiff(src string) string {
	var b strings.Builder
	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(line, "-") {
			b.WriteString(color.RedString(line) + "\n")
		} else if strings.HasPrefix(line, "+") {
			b.WriteString(color.GreenString(line) + "\n")
		} else {
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}

// APIClient represents a client for communicating with trabbits API server via Unix socket
type APIClient struct {
	endpoint string
	client   *http.Client
}

// NewAPIClient creates a new API client that communicates via Unix socket
func NewAPIClient(socketPath string) *APIClient {
	tr := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   30 * time.Second,
	}
	return &APIClient{
		endpoint: "http://localhost/",
		client:   client,
	}
}

// buildURL constructs a full URL from the endpoint and path
func (c *APIClient) buildURL(path string) (*url.URL, error) {
	baseURL, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	u, err := url.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("invalid path %q: %w", path, err)
	}

	return baseURL.ResolveReference(u), nil
}

func (c *APIClient) getConfig(ctx context.Context) (*config.Config, error) {
	fullURL, err := c.buildURL("config")
	if err != nil {
		return nil, err
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fullURL.String(), nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get config: %s", resp.Status)
	}
	var cfg config.Config
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}
	return &cfg, nil
}

func (c *APIClient) putConfigFromFile(ctx context.Context, configPath string) error {
	slog.Info("putting config from file", "file", configPath)

	// Read raw file content without any processing
	// Let the server handle environment variable expansion and Jsonnet evaluation
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	fullURL, err := c.buildURL("config")
	if err != nil {
		return err
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, fullURL.String(), bytes.NewReader(data))
	req.Header.Set("Content-Type", APIContentType)
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to put config: %s", resp.Status)
	}
	slog.Info("config updated successfully")
	return nil
}

// Keep the old method for backward compatibility, though it's not used anymore
func (c *APIClient) putConfig(ctx context.Context, cfg *config.Config) error {
	slog.Info("putting config", "config", cfg)
	b := new(bytes.Buffer)
	b.WriteString(cfg.String())
	fullURL, err := c.buildURL("config")
	if err != nil {
		return err
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, fullURL.String(), b)
	req.Header.Set("Content-Type", APIContentType)
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to put config: %s", resp.Status)
	}
	slog.Info("config updated successfully")
	return nil
}

func (c *APIClient) diffConfigFromFile(ctx context.Context, configPath string) (string, error) {
	slog.Info("getting diff from file", "file", configPath)

	// Read raw file content
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read config file: %w", err)
	}

	// Determine content type based on file extension
	contentType := APIContentType
	if strings.HasSuffix(configPath, ".jsonnet") {
		contentType = APIContentTypeJsonnet
	}

	fullURL, err := c.buildURL("config/diff")
	if err != nil {
		return "", err
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, fullURL.String(), bytes.NewReader(data))
	req.Header.Set("Content-Type", contentType)
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get diff: %s", resp.Status)
	}

	// Read the diff response
	diffBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read diff response: %w", err)
	}

	return string(diffBytes), nil
}

func (c *APIClient) reloadConfig(ctx context.Context) (*config.Config, error) {
	slog.Info("reloading config from server")

	fullURL, err := c.buildURL("config/reload")
	if err != nil {
		return nil, err
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, fullURL.String(), nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to reload config: %s", resp.Status)
	}

	var cfg config.Config
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}
	return &cfg, nil
}

func manageClients(ctx context.Context, opt *CLI) error {
	client := NewAPIClient(opt.APISocket)
	clients, err := client.GetClients(ctx)
	if err != nil {
		return fmt.Errorf("failed to get clients: %w", err)
	}

	// Pretty print the clients information
	clientsJSON, err := json.MarshalIndent(clients, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal clients: %w", err)
	}

	fmt.Print(string(clientsJSON))
	return nil
}

func (c *APIClient) GetClients(ctx context.Context) ([]types.ClientInfo, error) {
	fullURL, err := c.buildURL("clients")
	if err != nil {
		return nil, err
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fullURL.String(), nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get clients: %s", resp.Status)
	}

	var clients []types.ClientInfo
	if err := json.NewDecoder(resp.Body).Decode(&clients); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}
	return clients, nil
}

func manageProxyShutdown(ctx context.Context, opt *CLI) error {
	client := NewAPIClient(opt.APISocket)
	proxyID := opt.Manage.Clients.Shutdown.ProxyID
	reason := opt.Manage.Clients.Shutdown.Reason

	return client.shutdownProxy(ctx, proxyID, reason)
}

func (c *APIClient) shutdownProxy(ctx context.Context, proxyID, reason string) error {
	if proxyID == "" {
		return fmt.Errorf("proxy ID cannot be empty")
	}

	// Build URL properly using net/url
	fullURL, err := c.buildURL(path.Join("clients", proxyID))
	if err != nil {
		return err
	}

	if reason != "" {
		query := fullURL.Query()
		query.Set("reason", reason)
		fullURL.RawQuery = query.Encode()
	}

	// Debug: log the request details
	slog.Debug("shutdown request", "url", fullURL.String(), "method", "DELETE", "proxy_id", proxyID)

	req, _ := http.NewRequestWithContext(ctx, http.MethodDelete, fullURL.String(), nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusBadRequest {
		// Read response body for more details
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("bad request (failed to read response): %s (URL: %s)", resp.Status, fullURL.String())
		}
		bodyStr := strings.TrimSpace(string(body))
		if bodyStr == "" {
			bodyStr = "no error message"
		}
		return fmt.Errorf("bad request: %s (URL: %s, Status: %s)", bodyStr, fullURL.String(), resp.Status)
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("proxy not found: %s", proxyID)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to shutdown proxy: %s", resp.Status)
	}

	fmt.Printf("Proxy %s shutdown initiated successfully\n", proxyID)
	if reason != "" {
		fmt.Printf("Reason: %s\n", reason)
	}
	return nil
}

func manageProxyInfo(ctx context.Context, opt *CLI) error {
	client := NewAPIClient(opt.APISocket)
	proxyID := opt.Manage.Clients.Info.ProxyID

	return client.getProxyInfo(ctx, proxyID)
}

func (c *APIClient) GetClientDetail(ctx context.Context, clientID string) (*types.FullClientInfo, error) {
	fullURL, err := c.buildURL(path.Join("clients", clientID))
	if err != nil {
		return nil, err
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fullURL.String(), nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("client not found: %s", clientID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get client info: %s", resp.Status)
	}

	var clientInfo types.FullClientInfo
	if err := json.NewDecoder(resp.Body).Decode(&clientInfo); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &clientInfo, nil
}

func (c *APIClient) ShutdownClient(ctx context.Context, clientID, reason string) error {
	if clientID == "" {
		return fmt.Errorf("client ID cannot be empty")
	}

	// Build URL properly using net/url
	fullURL, err := c.buildURL(path.Join("clients", clientID))
	if err != nil {
		return err
	}

	if reason != "" {
		query := fullURL.Query()
		query.Set("reason", reason)
		fullURL.RawQuery = query.Encode()
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodDelete, fullURL.String(), nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusBadRequest {
		// Read response body for more details
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("bad request (failed to read response): %s", resp.Status)
		}
		bodyStr := strings.TrimSpace(string(body))
		if bodyStr == "" {
			bodyStr = "no error message"
		}
		return fmt.Errorf("bad request: %s", bodyStr)
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("client not found: %s", clientID)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to shutdown client: %s", resp.Status)
	}

	return nil
}

func (c *APIClient) getProxyInfo(ctx context.Context, proxyID string) error {
	fullURL, err := c.buildURL(path.Join("clients", proxyID))
	if err != nil {
		return err
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fullURL.String(), nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("proxy not found: %s", proxyID)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to get proxy info: %s", resp.Status)
	}

	// Parse and pretty-print JSON response for automation
	var clientInfo map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&clientInfo); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	// Output indented JSON for better readability
	clientJSON, err := json.MarshalIndent(clientInfo, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal client info: %w", err)
	}

	fmt.Print(string(clientJSON))
	return nil
}

// runTUI starts the TUI using the new tui package
func runTUI(ctx context.Context, opt *CLI) error {
	client := NewAPIClient(opt.APISocket)
	return tui.Run(ctx, client)
}

// manageProxyProbe streams real-time probe logs for a specific proxy
func manageProxyProbe(ctx context.Context, opt *CLI) error {
	client := NewAPIClient(opt.APISocket)
	proxyID := opt.Manage.Clients.Probe.ProxyID
	format := opt.Manage.Clients.Probe.Format

	slog.Info("Starting probe log stream", "proxy_id", proxyID, "format", format)

	return client.streamProbeLog(ctx, proxyID, format)
}

// ProbeLogEntry represents a parsed probe log entry
type ProbeLogEntry struct {
	Timestamp time.Time      `json:"timestamp"`
	Message   string         `json:"message"`
	Attrs     map[string]any `json:"attrs,omitempty"`
}

// streamProbeLog streams real-time probe logs from the API via SSE for CLI
func (c *APIClient) streamProbeLog(ctx context.Context, proxyID, format string) error {
	if proxyID == "" {
		return fmt.Errorf("proxy ID cannot be empty")
	}

	fmt.Printf("Connected to probe stream for proxy: %s\n", proxyID)
	fmt.Println("Press Ctrl+C to stop...")

	// Use the unified SSE reader
	return c.readProbeLogSSE(ctx, proxyID, func(entry *ProbeLogEntry) error {
		// Format for CLI display
		return c.formatProbeLogEntry(entry, format)
	})
}

// readProbeLogSSE reads SSE stream and calls handler for each probe log entry
func (c *APIClient) readProbeLogSSE(ctx context.Context, proxyID string, handler func(*ProbeLogEntry) error) error {
	fullURL, err := c.buildURL(fmt.Sprintf("clients/%s/probe", proxyID))
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set SSE headers
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	// Create a new client with no timeout for SSE streaming
	streamClient := c.client
	originalTimeout := c.client.Timeout
	c.client.Timeout = 0 // No timeout for streaming
	defer func() {
		c.client.Timeout = originalTimeout // Restore original timeout
	}()

	resp, err := streamClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to get probe stream: %s", resp.Status)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			// Context cancelled (Ctrl+C) - this is expected, not an error
			return nil
		default:
		}

		line := scanner.Text()

		// Skip empty lines and non-data lines
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}

		// Extract JSON data
		data := strings.TrimPrefix(line, "data: ")

		// Handle control messages
		if strings.Contains(data, `"type":"connected"`) {
			fmt.Println("✓ Connected to probe stream")
			continue
		}
		if strings.Contains(data, `"type":"proxy_ended"`) {
			fmt.Println("✗ Proxy connection ended")
			return nil
		}

		// Parse probe log
		var entry ProbeLogEntry
		if err := json.Unmarshal([]byte(data), &entry); err != nil {
			slog.Warn("Failed to parse probe log JSON", "error", err, "data", data)
			continue // Skip invalid logs
		}

		// Call handler
		if err := handler(&entry); err != nil {
			slog.Warn("Handler failed for probe log", "error", err)
		}
	}

	if err := scanner.Err(); err != nil {
		// Check if this is a context cancellation error
		if ctx.Err() != nil {
			return nil // Context cancelled - not an error
		}
		return fmt.Errorf("error reading stream: %w", err)
	}

	return nil
}

// formatProbeLogEntry formats and displays a probe log entry
func (c *APIClient) formatProbeLogEntry(entry *ProbeLogEntry, format string) error {
	if format == "json" {
		// Re-marshal to JSON for consistent output
		data, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	// Format as human-readable text
	timestamp := entry.Timestamp.Format("15:04:05.000")
	output := fmt.Sprintf("%s %s", timestamp, entry.Message)

	// Add attributes with consistent ordering
	if len(entry.Attrs) > 0 {
		// Sort keys to ensure consistent display order
		var keys []string
		for k := range entry.Attrs {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		var attrs []string
		for _, k := range keys {
			attrs = append(attrs, fmt.Sprintf("%s=%v", k, entry.Attrs[k]))
		}
		output += " " + strings.Join(attrs, " ")
	}

	fmt.Println(output)
	return nil
}

// StreamProbeLog streams probe logs for TUI interface
func (c *APIClient) StreamProbeLog(ctx context.Context, proxyID string) (<-chan tui.ProbeLogEntry, error) {
	logChan := make(chan tui.ProbeLogEntry, 100)
	slog.Debug("StreamProbeLog starting", "proxy_id", proxyID)

	go func() {
		defer close(logChan)

		// Use the unified SSE reader
		err := c.readProbeLogSSE(ctx, proxyID, func(entry *ProbeLogEntry) error {
			// Convert to TUI format and send to channel
			tuiEntry := tui.ProbeLogEntry{
				Timestamp: entry.Timestamp,
				Message:   entry.Message,
				Attrs:     entry.Attrs,
			}

			select {
			case logChan <- tuiEntry:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})

		if err != nil && ctx.Err() == nil {
			slog.Error("Probe stream error", "error", err)
		}
	}()

	return logChan, nil
}
