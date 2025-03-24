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

	Config  string `help:"Path to the configuration file." default:"config.json" env:"TRABBITS_CONFIG"`
	Port    int    `help:"Port to listen on." default:"6672" env:"TRABBITS_PORT"`
	APIPort int    `help:"Port to listen on for API (metrics, config and etc)." default:"16692" env:"TRABBITS_API_PORT"`

	Debug       bool             `help:"Enable debug mode." env:"DEBUG"`
	EnablePprof bool             `help:"Enable pprof." env:"ENABLE_PPROF"`
	Version     kong.VersionFlag `help:"Show version."`
}

type RunOptions struct {
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
	case "manage config <command>":
		// Manage the server
		return manageConfig(ctx, &cli)
	default:
		return fmt.Errorf("unknown command: %s", k.Command())
	}
}

func setupLogger(level slog.Level) {
	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))
}

type ManageOptions struct {
	Config struct {
		Command string `arg:"" enum:"get,diff,put" help:"Command to run."`
	} `cmd:"" help:"Manage the configuration."`
}
