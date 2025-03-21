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
	Run *RunOptions `cmd:"" help:"Run the trabbits server."`

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
	setupLogger(cli.Debug)

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
