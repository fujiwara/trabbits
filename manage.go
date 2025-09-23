package trabbits

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/fujiwara/trabbits/apiclient"
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
		return fmt.Errorf("unknown config command: %s", opt.Manage.Config.Command)
	}
}

func manageConfigGet(ctx context.Context, opt *CLI) error {
	client := apiclient.New(opt.APISocket)
	cfg, err := client.GetConfig(ctx)
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

	client := apiclient.New(opt.APISocket)
	diff, err := client.DiffConfigFromFile(ctx, opt.Manage.Config.File)
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

	client := apiclient.New(opt.APISocket)
	return client.PutConfigFromFile(ctx, opt.Manage.Config.File)
}

func manageConfigReload(ctx context.Context, opt *CLI) error {
	client := apiclient.New(opt.APISocket)
	cfg, err := client.ReloadConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to reload config: %w", err)
	}

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

func manageClients(ctx context.Context, opt *CLI) error {
	client := apiclient.New(opt.APISocket)
	clients, err := client.GetClients(ctx)
	if err != nil {
		return fmt.Errorf("failed to get clients: %w", err)
	}

	// Sort clients by ID for consistent output
	sort.Slice(clients, func(i, j int) bool {
		return clients[i].ID < clients[j].ID
	})

	for _, client := range clients {
		fmt.Printf("ID: %s, User: %s, Address: %s, Status: %s\n",
			client.ID, client.User, client.ClientAddress, client.Status)
	}
	return nil
}

func manageProxyShutdown(ctx context.Context, opt *CLI) error {
	client := apiclient.New(opt.APISocket)
	proxyID := opt.Manage.Clients.Shutdown.ProxyID
	reason := opt.Manage.Clients.Shutdown.Reason

	return client.ShutdownClient(ctx, proxyID, reason)
}

func manageProxyInfo(ctx context.Context, opt *CLI) error {
	client := apiclient.New(opt.APISocket)
	proxyID := opt.Manage.Clients.Info.ProxyID

	clientInfo, err := client.GetClientDetail(ctx, proxyID)
	if err != nil {
		return fmt.Errorf("failed to get client detail: %w", err)
	}

	fmt.Printf("Proxy ID: %s\n", clientInfo.ID)
	fmt.Printf("Client Address: %s\n", clientInfo.ClientAddress)
	fmt.Printf("Virtual Host: %s\n", clientInfo.VirtualHost)
	fmt.Printf("User: %s\n", clientInfo.User)
	fmt.Printf("Connected At: %s\n", clientInfo.ConnectedAt.Format("2006-01-02 15:04:05"))

	return nil
}

// runTUI starts the TUI using the new tui package
func runTUI(ctx context.Context, opt *CLI) error {
	client := apiclient.New(opt.APISocket)
	return tui.Run(ctx, client)
}

// manageProxyProbe streams real-time probe logs for a specific proxy
func manageProxyProbe(ctx context.Context, opt *CLI) error {
	client := apiclient.New(opt.APISocket)
	proxyID := opt.Manage.Clients.Probe.ProxyID
	format := opt.Manage.Clients.Probe.Format

	if format == "" {
		format = "text"
	}

	logChan, err := client.StreamProbeLogEntries(ctx, proxyID)
	if err != nil {
		return err
	}

	for entry := range logChan {
		if err := formatProbeLogEntry(&entry, format); err != nil {
			return err
		}
	}
	return nil
}

// formatProbeLogEntry formats and displays a probe log entry
func formatProbeLogEntry(entry *types.ProbeLogEntry, format string) error {
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
