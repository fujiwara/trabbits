package trabbits

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const APIContentType = "application/json"

func runAPIServer(ctx context.Context, opt *CLI) (func(), error) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("GET /config", apiGetConfigHandler(opt))
	mux.HandleFunc("PUT /config", apiPutConfigHandler(opt))
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

func apiGetConfigHandler(opt *CLI) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", APIContentType)
		cfg := mustGetConfig()
		json.NewEncoder(w).Encode(cfg)
	})
}

func apiPutConfigHandler(opt *CLI) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, APIContentType) {
			slog.Error("Content-Type must be "+APIContentType, "content-type", ct)
			http.Error(w, "invalid Content-Type: "+ct, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		tmpfile, err := os.CreateTemp("", "trabbits-config-*.json")
		if err != nil {
			slog.Error("failed to create temporary file", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		configFile := tmpfile.Name()
		defer func() {
			os.Remove(configFile)
		}()
		if n, err := io.Copy(tmpfile, r.Body); err != nil {
			slog.Error("failed to write to temporary file", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		} else {
			slog.Info("configuration received", "size", n, "file", configFile)
		}
		cfg, err := LoadConfig(configFile)
		if err != nil {
			slog.Error("failed to load configuration", "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest) // payload is invalid
			return
		}
		storeConfig(cfg)
		json.NewEncoder(w).Encode(cfg)
	})
}
