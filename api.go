package trabbits

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func runAPIServer(ctx context.Context, opt *CLI) (func(), error) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	var srv http.Server
	// start API server
	go func() {
		slog.Info("starting API server", "port", opt.APIPort)
		srv := &http.Server{
			Handler: mux,
			Addr:    fmt.Sprintf(":%d", opt.APIPort),
		}
		if err := srv.ListenAndServe(); err != nil {
			slog.Error("failed to start API server", "error", err)
		}
	}()
	return func() { srv.Shutdown(ctx) }, nil
}
