// MIT License
// Copyright (c) 2025 FUJIWARA Shunichiro

package trabbits

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	_ "net/http/pprof"

	"github.com/alecthomas/kong"
)

type CLI struct {
	Run    *RunOptions    `cmd:"" help:"Run the trabbits server."`
	Manage *ManageOptions `cmd:"" help:"Manage the trabbits server."`
	Test   *TestOptions   `cmd:"" help:"Test utilities for trabbits."`

	Config      string `help:"Path to the configuration file." default:"config.json" env:"TRABBITS_CONFIG"`
	Port        int    `help:"Port to listen on." default:"6672" env:"TRABBITS_PORT"`
	MetricsPort int    `help:"Port to listen on for metrics" default:"16692" env:"TRABBITS_METRICS_PORT"`
	APISocket   string `help:"Path to the API socket." default:"/tmp/trabbits.sock" env:"TRABBITS_API_SOCKET"`

	Debug       bool             `help:"Enable debug mode." env:"DEBUG"`
	EnablePprof bool             `help:"Enable pprof." env:"ENABLE_PPROF"`
	Version     kong.VersionFlag `help:"Show version."`
}

type RunOptions struct {
	PidFile string `help:"Path to write the process ID file." env:"TRABBITS_PID_FILE"`
}

func Run(ctx context.Context) error {
	var cli CLI
	k := kong.Parse(&cli, kong.Vars{"version": fmt.Sprintf("trabbits %s", Version)})
	var logLevel = slog.LevelInfo
	if cli.Debug {
		logLevel = slog.LevelDebug
	}
	setupLogger(logLevel)

	if cli.EnablePprof {
		go func() {
			err := http.ListenAndServe("localhost:6060", nil)
			if err != nil {
				panic(fmt.Sprintf("failed to start pprof: %v", err))
			}
		}()
	}

	switch k.Command() {
	case "run":
		// Run the server
		return run(ctx, &cli)
	case "manage config <command>", "manage config <command> <file>":
		// Manage the server
		return manageConfig(ctx, &cli)
	case "manage clients list":
		// Get clients information
		return manageClients(ctx, &cli)
	case "manage clients tui":
		// Interactive TUI for managing clients
		return runTUI(ctx, &cli)
	case "manage clients shutdown <proxy-id>":
		// Shutdown a specific proxy
		return manageProxyShutdown(ctx, &cli)
	case "manage clients info <proxy-id>":
		// Get detailed information for a specific proxy
		return manageProxyInfo(ctx, &cli)
	case "manage clients probe <proxy-id>":
		// Stream real-time probe logs for a specific proxy
		return manageProxyProbe(ctx, &cli)
	case "test match-routing <pattern> <key>":
		// Test routing pattern matching
		return testMatchRouting(ctx, &cli)
	default:
		return fmt.Errorf("unknown command: %s", k.Command())
	}
}

func setupLogger(level slog.Level) {
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	mh := NewMetricSlogHandler(h, GetMetrics().LoggerStats)
	slog.SetDefault(slog.New(mh))
}

type ManageOptions struct {
	Config struct {
		Command string `arg:"" enum:"get,diff,put,reload" help:"Command to run (get, diff, put, reload)."`
		File    string `arg:"" optional:"" help:"Configuration file (required for diff/put commands)."`
	} `cmd:"" help:"Manage the configuration."`
	Clients struct {
		List     struct{} `cmd:"" help:"Get connected clients information."`
		TUI      struct{} `cmd:"" default:"true" help:"Interactive TUI for managing clients (default)."`
		Shutdown struct {
			ProxyID string `arg:"" required:"" help:"Proxy ID to shutdown."`
			Reason  string `help:"Optional shutdown reason."`
		} `cmd:"" help:"Shutdown a specific proxy."`
		Info struct {
			ProxyID string `arg:"" required:"" help:"Proxy ID to get detailed information for."`
		} `cmd:"" help:"Get detailed information for a specific proxy."`
		Probe struct {
			ProxyID string `arg:"" required:"" help:"Proxy ID to monitor probe logs for."`
			Format  string `help:"Output format (json|text)." default:"text" enum:"json,text"`
		} `cmd:"" help:"Stream real-time probe logs for a specific proxy."`
	} `cmd:"" help:"Manage connected clients."`
}

type TestOptions struct {
	MatchRouting struct {
		Pattern string `arg:"" required:"" help:"Binding pattern to test (e.g., 'logs.*.error', 'metrics.#')."`
		Key     string `arg:"" required:"" help:"Routing key to match against the pattern."`
	} `cmd:"" help:"Test routing pattern matching."`
}
