package trabbits

import (
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
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/fujiwara/trabbits/config"
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
	client := newAPIClient(opt.APISocket)
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

	client := newAPIClient(opt.APISocket)
	diff, err := client.diffConfigFromFile(ctx, opt.Manage.Config.File)
	if err != nil {
		return fmt.Errorf("failed to get diff: %w", err)
	}

	if diff != "" {
		fmt.Print(coloredDiff(diff))
	}

	return nil
}

func manageConfigPut(ctx context.Context, opt *CLI) error {
	if opt.Manage.Config.File == "" {
		return fmt.Errorf("configuration file is required for put command")
	}

	client := newAPIClient(opt.APISocket)
	return client.putConfigFromFile(ctx, opt.Manage.Config.File)
}

func manageConfigReload(ctx context.Context, opt *CLI) error {
	client := newAPIClient(opt.APISocket)
	cfg, err := client.reloadConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to reload config: %w", err)
	}
	slog.Info("Configuration reloaded successfully")
	fmt.Print(cfg.String())
	return nil
}

func coloredDiff(src string) string {
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

type apiClient struct {
	endpoint string
	client   *http.Client
}

func newAPIClient(socketPath string) *apiClient {
	tr := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   30 * time.Second,
	}
	return &apiClient{
		endpoint: "http://localhost/",
		client:   client,
	}
}

// buildURL constructs a full URL from the endpoint and path
func (c *apiClient) buildURL(path string) (*url.URL, error) {
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

func (c *apiClient) getConfig(ctx context.Context) (*config.Config, error) {
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

func (c *apiClient) putConfigFromFile(ctx context.Context, configPath string) error {
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
func (c *apiClient) putConfig(ctx context.Context, cfg *config.Config) error {
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

func (c *apiClient) diffConfigFromFile(ctx context.Context, configPath string) (string, error) {
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

func (c *apiClient) reloadConfig(ctx context.Context) (*config.Config, error) {
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
	client := newAPIClient(opt.APISocket)
	clients, err := client.getClients(ctx)
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

func (c *apiClient) getClients(ctx context.Context) ([]types.ClientInfo, error) {
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
	client := newAPIClient(opt.APISocket)
	proxyID := opt.Manage.Clients.Shutdown.ProxyID
	reason := opt.Manage.Clients.Shutdown.Reason

	return client.shutdownProxy(ctx, proxyID, reason)
}

func (c *apiClient) shutdownProxy(ctx context.Context, proxyID, reason string) error {
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
	client := newAPIClient(opt.APISocket)
	proxyID := opt.Manage.Clients.Info.ProxyID

	return client.getProxyInfo(ctx, proxyID)
}

func (c *apiClient) getClientDetail(ctx context.Context, clientID string) (*types.FullClientInfo, error) {
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

func (c *apiClient) shutdownClient(ctx context.Context, clientID, reason string) error {
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

func (c *apiClient) getProxyInfo(ctx context.Context, proxyID string) error {
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
