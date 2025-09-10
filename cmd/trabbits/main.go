package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"

	"github.com/fujiwara/trabbits"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	// Setup signal handling with logging
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, signals()...)

	go func() {
		sig := <-sigChan
		slog.Info("Received signal, shutting down", "signal", sig)
		cancel()
	}()

	if err := run(ctx); err != nil {
		slog.Error("failed to run", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	return trabbits.Run(ctx)
}
