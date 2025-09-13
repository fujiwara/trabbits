package trabbits_test

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestSIGHUPSignalHandling tests actual SIGHUP signal handling
func TestSIGHUPSignalHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create initial config file
	initialConfig := `{
		"upstreams": [
			{
				"name": "primary",
				"address": "localhost:5672",
				"routing": {}
			}
		]
	}`

	configFile, err := os.CreateTemp("", "trabbits-sighup-integration-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp config file: %v", err)
	}
	defer os.Remove(configFile.Name())

	if _, err := configFile.Write([]byte(initialConfig)); err != nil {
		t.Fatalf("Failed to write initial config: %v", err)
	}
	configFile.Close()

	// Create API socket path
	apiSocket, err := os.CreateTemp("", "trabbits-sighup-api-*.sock")
	if err != nil {
		t.Fatalf("Failed to create temp API socket file: %v", err)
	}
	apiSocketPath := apiSocket.Name()
	apiSocket.Close()
	os.Remove(apiSocketPath) // trabbits will create the socket

	// Start trabbits process
	cmd := exec.Command("go", "run", "./cmd/trabbits", "run",
		"--config", configFile.Name(),
		"--api-socket", apiSocketPath,
		"--port", "0", // Use random port
		"--metrics-port", "0") // Use random metrics port

	// Capture stderr for debugging
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("Failed to create stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start trabbits process: %v", err)
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
		os.Remove(apiSocketPath)
	}()

	// Read stderr in background for debugging
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if err != nil {
				return
			}
			// Print each line separately for better readability
			lines := strings.Split(strings.TrimSpace(string(buf[:n])), "\n")
			for _, line := range lines {
				if line != "" {
					t.Logf("trabbits: %s", line)
				}
			}
		}
	}()

	// Wait for server to start
	time.Sleep(5 * time.Second)

	// Verify initial config via API
	initialCfg := getConfigViaAPI(t, apiSocketPath)
	if len(initialCfg.Upstreams) != 1 || initialCfg.Upstreams[0].Name != "primary" {
		t.Fatalf("Initial config verification failed: %+v", initialCfg)
	}
	t.Logf("✓ Initial config loaded correctly")

	// Update config file
	updatedConfig := `{
		"upstreams": [
			{
				"name": "primary",
				"address": "localhost:5672",
				"routing": {}
			},
			{
				"name": "sighup-added",
				"address": "localhost:5673",
				"routing": {
					"key_patterns": ["sighup.test.*"]
				}
			}
		]
	}`

	if err := os.WriteFile(configFile.Name(), []byte(updatedConfig), 0644); err != nil {
		t.Fatalf("Failed to update config file: %v", err)
	}
	t.Logf("✓ Config file updated")

	// Check process is still running before sending signal
	if cmd.Process == nil {
		t.Fatalf("Process is nil")
	}
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		t.Fatalf("Process has already exited")
	}
	t.Logf("Process PID: %d, sending SIGHUP...", cmd.Process.Pid)

	// Send SIGHUP signal to the process
	if err := cmd.Process.Signal(syscall.SIGHUP); err != nil {
		t.Fatalf("Failed to send SIGHUP signal: %v", err)
	}
	t.Logf("✓ SIGHUP signal sent to process (PID: %d)", cmd.Process.Pid)

	// Wait longer for config reload and monitor logs
	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)
		t.Logf("Waiting for config reload... (%d/10)", i+1)
	}

	// Verify config was reloaded via API
	reloadedCfg := getConfigViaAPI(t, apiSocketPath)
	if len(reloadedCfg.Upstreams) != 2 {
		t.Fatalf("Expected 2 upstreams after SIGHUP, got %d", len(reloadedCfg.Upstreams))
	}

	// Check if the new upstream exists
	foundSighupAdded := false
	for _, upstream := range reloadedCfg.Upstreams {
		if upstream.Name == "sighup-added" {
			foundSighupAdded = true
			if len(upstream.Routing.KeyPatterns) != 1 || upstream.Routing.KeyPatterns[0] != "sighup.test.*" {
				t.Errorf("Unexpected routing patterns for sighup-added upstream: %v", upstream.Routing.KeyPatterns)
			}
			break
		}
	}

	if !foundSighupAdded {
		t.Error("Expected to find 'sighup-added' upstream after SIGHUP")
	}

	t.Logf("✓ SIGHUP signal successfully triggered config reload")
	t.Logf("✓ New upstream 'sighup-added' found with correct routing patterns")
}

// Config struct for API response (simplified version)
type Config struct {
	Upstreams []Upstream `json:"upstreams"`
}

type Upstream struct {
	Name    string  `json:"name"`
	Address string  `json:"address"`
	Routing Routing `json:"routing"`
}

type Routing struct {
	KeyPatterns []string `json:"key_patterns"`
}

// getConfigViaAPI retrieves current config via Unix socket API
func getConfigViaAPI(t *testing.T, socketPath string) *Config {
	t.Helper()

	// Wait for API socket to be available
	for i := 0; i < 50; i++ { // 5 seconds max
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	}

	resp, err := client.Get("http://unix/config")
	if err != nil {
		t.Fatalf("Failed to get config via API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var cfg Config
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		t.Fatalf("Failed to decode config response: %v", err)
	}

	return &cfg
}