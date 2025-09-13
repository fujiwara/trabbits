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
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/fujiwara/trabbits/config"
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
		endpoint: "http://localhost/config",
		client:   client,
	}
}

func (c *apiClient) getConfig(ctx context.Context) (*config.Config, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
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

	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, c.endpoint, bytes.NewReader(data))
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
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, c.endpoint, b)
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

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/diff", bytes.NewReader(data))
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

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/reload", nil)
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
