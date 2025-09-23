package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/fujiwara/trabbits/config"
)

// GetConfig retrieves the current configuration from the server
func (c *Client) GetConfig(ctx context.Context) (*config.Config, error) {
	fullURL, err := c.buildURL("config")
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fullURL.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var cfg config.Config
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// PutConfigFromFile uploads configuration from a file to the server
func (c *Client) PutConfigFromFile(ctx context.Context, configPath string) error {
	slog.Info("putting config from file", "file", configPath)

	// Read raw file content without any processing
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	fullURL, err := c.buildURL("config")
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", fullURL.String(), bytes.NewReader(configBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to put config: %s - %s", resp.Status, string(body))
	}

	return nil
}

// PutConfig uploads configuration to the server (kept for backward compatibility)
func (c *Client) PutConfig(ctx context.Context, cfg *config.Config) error {
	slog.Info("putting config", "config", cfg)
	b := new(bytes.Buffer)
	b.WriteString(cfg.String())

	fullURL, err := c.buildURL("config")
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", fullURL.String(), b)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// DiffConfigFromFile gets configuration diff between file and server
func (c *Client) DiffConfigFromFile(ctx context.Context, configPath string) (string, error) {
	slog.Info("getting diff from file", "file", configPath)

	// Read raw file content
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read config file: %w", err)
	}

	fullURL, err := c.buildURL("config/diff")
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fullURL.String(), bytes.NewReader(configBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get diff: %s - %s", resp.Status, string(body))
	}

	diffBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(diffBytes), nil
}

// ReloadConfig reloads configuration on the server
func (c *Client) ReloadConfig(ctx context.Context) (*config.Config, error) {
	slog.Info("reloading config from server")

	fullURL, err := c.buildURL("config/reload")
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fullURL.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var cfg config.Config
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
