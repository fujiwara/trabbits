// MIT License
// Copyright (c) 2025 FUJIWARA Shunichiro

package trabbits

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/alecthomas/kong"
)

type CLI struct {
	Run *RunOptions `cmd:"" help:"Run the trabbits server."`

	Debug   bool             `help:"Enable debug mode." env:"DEBUG"`
	Version kong.VersionFlag `help:"Show version."`
}

type RunOptions struct {
	Port   int    `help:"Port to listen on." default:"5673" env:"TRABBITS_PORT"`
	Config string `help:"Path to the configuration file." default:"config.json" env:"TRABBITS_CONFIG"`
}

func Run(ctx context.Context) error {
	var cli CLI
	k := kong.Parse(&cli, kong.Vars{"version": fmt.Sprintf("trabbits %s", Version)})
	setupLogger(cli.Debug)

	switch k.Command() {
	case "run":
		// Run the server
		return run(ctx, cli.Run)
	}
	return nil
}

func setupLogger(debug bool) {
	var level slog.Level
	if debug {
		level = slog.LevelDebug
	} else {
		level = slog.LevelInfo
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))
}
