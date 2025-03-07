package main

import (
	"context"
	"log"
	"os/signal"

	"github.com/fujiwara/trabbits"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), signals()...)
	defer stop()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	return trabbits.Run(ctx)
}
