package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"

	"github.com/fujiwara/trabbits"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), signals()...)
	defer stop()
	if err := run(ctx); err != nil {
		slog.Error("failed to run", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	return trabbits.Run(ctx)
}
