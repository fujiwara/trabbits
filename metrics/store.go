package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Store manages metrics and their registry for a server instance
type Store struct {
	metrics  *Metrics
	registry *prometheus.Registry
}

// NewStore creates a new metrics store with initialized metrics and registry
func NewStore() *Store {
	registry := prometheus.NewRegistry()
	metrics := NewMetrics()
	metrics.MustRegister(registry)

	return &Store{
		metrics:  metrics,
		registry: registry,
	}
}

// Metrics returns the metrics instance for this store
func (s *Store) Metrics() *Metrics {
	return s.metrics
}

// Registry returns the prometheus registry for this store
func (s *Store) Registry() *prometheus.Registry {
	return s.registry
}

// RunServer starts a metrics HTTP server for this store
func (s *Store) RunServer(ctx context.Context, port int) (func(), error) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{}))

	srv := &http.Server{
		Handler: mux,
		Addr:    fmt.Sprintf(":%d", port),
	}

	// Start server in goroutine
	ch := make(chan error, 1)
	go func() {
		slog.Info("starting metrics server", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("failed to start metrics server", "error", err)
			ch <- err
		}
	}()

	// Wait for server to start or fail
	wait := time.NewTimer(100 * time.Millisecond)
	select {
	case err := <-ch:
		return nil, err
	case <-wait.C:
		slog.Info("metrics server started", "port", port)
	}

	// Return shutdown function
	shutdown := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("failed to shutdown metrics server", "error", err)
		} else {
			slog.Info("metrics server stopped")
		}
	}

	return shutdown, nil
}