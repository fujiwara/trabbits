package trabbits

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
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
