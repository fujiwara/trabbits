package trabbits

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/aereal/jsondiff"
	"github.com/fatih/color"
)

func manageConfig(ctx context.Context, opt *CLI) error {
	switch opt.Manage.Config.Command {
	case "get":
		return manageConfigGet(ctx, opt)
	case "diff":
		return manageConfigDiff(ctx, opt)
	case "put":
		return manageConfigPut(ctx, opt)
	default:
		return fmt.Errorf("unknown command: %s", opt.Manage.Config.Command)
	}
}

func manageConfigGet(ctx context.Context, opt *CLI) error {
	client := newAPIClient(opt.APIPort)
	cfg, err := client.getConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}
	fmt.Print(cfg.String())
	return nil
}

func manageConfigDiff(ctx context.Context, opt *CLI) error {
	newCfg, err := LoadConfig(opt.Config)
	if err != nil {
		return err
	}

	client := newAPIClient(opt.APIPort)
	currentCfg, err := client.getConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	if diff, err := jsondiff.Diff(
		&jsondiff.Input{Name: client.endpoint, X: currentCfg},
		&jsondiff.Input{Name: opt.Config, X: newCfg},
		// opts...
	); err != nil {
		return fmt.Errorf("failed to diff: %w", err)
	} else if diff != "" {
		fmt.Print(coloredDiff(diff))
	}

	return nil
}

func manageConfigPut(ctx context.Context, opt *CLI) error {
	client := newAPIClient(opt.APIPort)
	newCfg, err := LoadConfig(opt.Config)
	if err != nil {
		return err
	}
	return client.putConfig(ctx, newCfg)
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

func newAPIClient(port int) *apiClient {
	endpoint := fmt.Sprintf("http://localhost:%d/config", port)
	return &apiClient{
		endpoint: endpoint,
		client:   http.DefaultClient,
	}
}

func (c *apiClient) getConfig(ctx context.Context) (*Config, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get config: %s", resp.Status)
	}
	var cfg Config
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}
	return &cfg, nil
}

func (c *apiClient) putConfig(ctx context.Context, cfg *Config) error {
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
