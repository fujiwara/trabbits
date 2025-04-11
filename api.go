package trabbits

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
)

const APIContentType = "application/json"

func listenUnixSocket(socketPath string) (net.Listener, func(), error) {
	if socketPath == "" {
		return nil, func() {}, fmt.Errorf("path to socket is empty")
	}
	if _, err := os.Stat(socketPath); err == nil {
		if err := os.Remove(socketPath); err != nil {
			return nil, func() {}, fmt.Errorf("failed to remove existing socket: %w", err)
		}
	}
	// remove the socket file when the server is stopped
	cancelFunc := func() {
		os.Remove(socketPath)
	}
	// Create a new Unix socket listener
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, cancelFunc, fmt.Errorf("failed to listen on socket: %w", err)
	}
	// Set the socket permissions to allow only the owner to read/write
	if err := os.Chmod(socketPath, 0600); err != nil {
		return nil, cancelFunc, fmt.Errorf("failed to set socket permissions: %w", err)
	}
	return listener, cancelFunc, nil
}

func runAPIServer(ctx context.Context, opt *CLI) (func(), error) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /config", apiGetConfigHandler(opt))
	mux.HandleFunc("PUT /config", apiPutConfigHandler(opt))
	var srv http.Server
	// start API server
	go func() {
		slog.Info("starting API server", "socket", opt.APISocket)
		listener, cancel, err := listenUnixSocket(opt.APISocket)
		defer cancel()
		if err != nil {
			slog.Error("failed to start API server", "error", err)
			return
		}
		srv := &http.Server{
			Handler: mux,
		}
		if err := srv.Serve(listener); err != nil {
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
