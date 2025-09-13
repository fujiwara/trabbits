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
	"time"

	"github.com/aereal/jsondiff"
	"github.com/fujiwara/trabbits/config"
)

const (
	APIContentType        = "application/json"
	APIContentTypeJsonnet = "application/jsonnet"
)

func listenUnixSocket(socketPath string) (net.Listener, func(), error) {
	if socketPath == "" {
		return nil, func() {}, fmt.Errorf("path to socket is empty")
	}
	if _, err := os.Stat(socketPath); err == nil {
		return nil, func() {}, fmt.Errorf("socket already exists: %s", socketPath)
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

// detectContentType checks the Content-Type header and returns whether it's Jsonnet
func detectContentType(ct string) (isJsonnet bool, err error) {
	if strings.HasPrefix(ct, APIContentTypeJsonnet) {
		return true, nil
	} else if strings.HasPrefix(ct, APIContentType) {
		return false, nil
	}
	return false, fmt.Errorf("invalid Content-Type: %s", ct)
}

// createTempConfigFile creates a temporary file for config based on content type
func createTempConfigFile(isJsonnet bool, prefix string) (*os.File, error) {
	var suffix string
	if isJsonnet {
		suffix = "*.jsonnet"
	} else {
		suffix = "*.json"
	}
	return os.CreateTemp("", prefix+suffix)
}

// processConfigRequest handles common config request processing
func processConfigRequest(w http.ResponseWriter, r *http.Request, prefix string) (string, error) {
	ct := r.Header.Get("Content-Type")
	isJsonnet, err := detectContentType(ct)
	if err != nil {
		slog.Error("Content-Type must be application/json or application/jsonnet", "content-type", ct)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return "", err
	}

	formatType := "JSON"
	if isJsonnet {
		formatType = "Jsonnet"
	}
	slog.Debug(fmt.Sprintf("API %s request received", strings.ToUpper(prefix)), "content-type", ct, "format", formatType)

	tmpfile, err := createTempConfigFile(isJsonnet, prefix)
	if err != nil {
		slog.Error("failed to create temporary file", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return "", err
	}

	configFile := tmpfile.Name()
	if n, err := io.Copy(tmpfile, r.Body); err != nil {
		tmpfile.Close()
		os.Remove(configFile)
		slog.Error("failed to write to temporary file", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return "", err
	} else {
		slog.Info("configuration received", "size", n, "file", configFile, "format", formatType)
	}
	tmpfile.Close()

	return configFile, nil
}

// startAPIServer starts the API server for this server instance
func (s *Server) startAPIServer(ctx context.Context, configPath string) (func(), error) {
	if s.apiSocket == "" {
		return func() {}, nil // No API server if socket not specified
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /config", s.apiGetConfigHandler())
	mux.HandleFunc("PUT /config", s.apiPutConfigHandler())
	mux.HandleFunc("POST /config/diff", s.apiDiffConfigHandler())
	mux.HandleFunc("POST /config/reload", s.apiReloadConfigHandler(configPath))
	var srv http.Server
	// start API server
	ch := make(chan error)
	go func() {
		slog.Info("starting API server", "socket", s.apiSocket)
		listener, cancel, err := listenUnixSocket(s.apiSocket)
		defer cancel()
		if err != nil {
			slog.Error("failed to listen API server socket", "error", err)
			ch <- err
			return
		}
		srv := &http.Server{
			Handler: mux,
		}
		if err := srv.Serve(listener); err != nil {
			slog.Error("failed to start API server", "error", err)
			ch <- err
		}
	}()

	wait := time.NewTimer(100 * time.Millisecond)
	select {
	case err := <-ch:
		return nil, err
	case <-wait.C:
		slog.Info("API server started", "socket", s.apiSocket)
	}
	return func() {
		os.Remove(s.apiSocket)
		srv.Shutdown(ctx)
	}, nil
}

// API handler methods for the server
func (s *Server) apiGetConfigHandler() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", APIContentType)
		cfg := s.GetConfig()
		json.NewEncoder(w).Encode(cfg)
	})
}

func (s *Server) apiPutConfigHandler() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		configFile, err := processConfigRequest(w, r, "trabbits-config-")
		if err != nil {
			return
		}
		defer os.Remove(configFile)

		w.Header().Set("Content-Type", "application/json")

		cfg, err := config.LoadConfig(r.Context(), configFile)
		if err != nil {
			slog.Error("failed to load configuration", "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest) // payload is invalid
			return
		}

		// Reinitialize health managers with new configuration
		if err := s.initHealthManagers(r.Context(), cfg); err != nil {
			slog.Error("failed to reinit health managers", "error", err)
			// Don't fail the config update, just log the error
		}

		// Update server config and disconnect outdated proxies
		s.UpdateConfig(cfg)
		disconnectChan := s.disconnectOutdatedProxies(cfg.Hash())
		go func() {
			disconnectedCount := <-disconnectChan
			if disconnectedCount > 0 {
				slog.Info("Completed disconnection of outdated proxies", "count", disconnectedCount)
			}
		}()

		json.NewEncoder(w).Encode(cfg)
	})
}

func (s *Server) apiDiffConfigHandler() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		configFile, err := processConfigRequest(w, r, "trabbits-config-diff-")
		if err != nil {
			return
		}
		defer os.Remove(configFile)

		w.Header().Set("Content-Type", "text/plain")

		// Load new config from request
		newCfg, err := config.LoadConfig(r.Context(), configFile)
		if err != nil {
			slog.Error("failed to load new configuration", "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Get current config
		currentCfg := s.GetConfig()

		// Generate diff using jsondiff
		diff, err := jsondiff.Diff(
			&jsondiff.Input{Name: "current", X: currentCfg},
			&jsondiff.Input{Name: "new", X: newCfg},
		)
		if err != nil {
			slog.Error("failed to generate diff", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Return diff as plain text
		w.Write([]byte(diff))
	})
}

func (s *Server) apiReloadConfigHandler(configPath string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		cfg, err := s.reloadConfigFromFile(r.Context(), configPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(cfg)
	})
}

// reloadConfigFromFile reloads configuration from the specified file
func (s *Server) reloadConfigFromFile(ctx context.Context, configPath string) (*config.Config, error) {
	slog.Info("Reloading configuration from file", "file", configPath)

	// Reload config from the original config file
	cfg, err := config.LoadConfig(ctx, configPath)
	if err != nil {
		slog.Error("failed to reload configuration", "error", err)
		return nil, fmt.Errorf("failed to reload configuration: %w", err)
	}

	// Reinitialize health managers with new configuration
	if err := s.initHealthManagers(ctx, cfg); err != nil {
		slog.Error("failed to reinit health managers", "error", err)
		// Don't fail the config reload, just log the error
	}

	// Update server config and disconnect outdated proxies
	s.UpdateConfig(cfg)
	disconnectChan := s.disconnectOutdatedProxies(cfg.Hash())
	go func() {
		disconnectedCount := <-disconnectChan
		if disconnectedCount > 0 {
			slog.Info("Completed disconnection of outdated proxies", "count", disconnectedCount)
		}
	}()

	slog.Info("Configuration reloaded successfully")
	return cfg, nil
}
