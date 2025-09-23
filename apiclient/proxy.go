package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"

	"github.com/fujiwara/trabbits/types"
)

// GetClients retrieves list of connected clients
func (c *Client) GetClients(ctx context.Context) ([]types.ClientInfo, error) {
	fullURL, err := c.buildURL("clients")
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fullURL.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var clients []types.ClientInfo
	if err := json.NewDecoder(resp.Body).Decode(&clients); err != nil {
		return nil, err
	}
	return clients, nil
}

// GetClientDetail retrieves detailed information about a specific client
func (c *Client) GetClientDetail(ctx context.Context, clientID string) (*types.FullClientInfo, error) {
	fullURL, err := c.buildURL(path.Join("clients", clientID))
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fullURL.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("client not found")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get client detail: %s - %s", resp.Status, string(body))
	}

	var clientInfo types.FullClientInfo
	if err := json.NewDecoder(resp.Body).Decode(&clientInfo); err != nil {
		return nil, err
	}
	return &clientInfo, nil
}

// ShutdownClient shuts down a specific client connection
func (c *Client) ShutdownClient(ctx context.Context, clientID, reason string) error {
	if clientID == "" {
		return fmt.Errorf("client ID cannot be empty")
	}

	if reason == "" {
		reason = "shutdown requested"
	}

	fullURL, err := c.buildURL(path.Join("clients", clientID, "shutdown"))
	if err != nil {
		return err
	}

	payload := map[string]string{
		"reason": reason,
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fullURL.String(), bytes.NewReader(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("client not found")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to shutdown client: %s - %s", resp.Status, string(body))
	}

	return nil
}

// shutdownProxy shuts down a specific proxy connection (internal method)
func (c *Client) shutdownProxy(ctx context.Context, proxyID, reason string) error {
	if proxyID == "" {
		return fmt.Errorf("proxy ID cannot be empty")
	}

	if reason == "" {
		reason = "shutdown requested"
	}

	// Create shutdown request payload
	shutdownData := struct {
		Reason string `json:"reason"`
	}{
		Reason: reason,
	}

	jsonData, err := json.Marshal(shutdownData)
	if err != nil {
		return fmt.Errorf("failed to marshal shutdown data: %w", err)
	}

	// Construct URL for proxy shutdown
	shutdownURL := fmt.Sprintf("clients/%s/shutdown", url.PathEscape(proxyID))
	fullURL, err := c.buildURL(shutdownURL)
	if err != nil {
		return fmt.Errorf("failed to build URL: %w", err)
	}

	// Create and send request
	req, err := http.NewRequestWithContext(ctx, "POST", fullURL.String(), bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("proxy not found: %s", proxyID)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("shutdown failed: %s - %s", resp.Status, string(body))
	}

	return nil
}

// getProxyInfo retrieves information about a specific proxy (internal method)
func (c *Client) getProxyInfo(ctx context.Context, proxyID string) error {
	fullURL, err := c.buildURL(path.Join("clients", proxyID))
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fullURL.String(), nil)
	if err != nil {
		return err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("proxy not found")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to get proxy info: %s - %s", resp.Status, string(body))
	}

	var proxyInfo types.FullClientInfo
	if err := json.NewDecoder(resp.Body).Decode(&proxyInfo); err != nil {
		return err
	}

	fmt.Printf("Proxy ID: %s\n", proxyInfo.ID)
	fmt.Printf("Client Address: %s\n", proxyInfo.ClientAddress)
	fmt.Printf("Virtual Host: %s\n", proxyInfo.VirtualHost)
	fmt.Printf("User: %s\n", proxyInfo.User)
	fmt.Printf("Connected At: %s\n", proxyInfo.ConnectedAt.Format("2006-01-02 15:04:05"))

	return nil
}
