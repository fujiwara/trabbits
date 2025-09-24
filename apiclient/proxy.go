package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

	fullURL, err := c.buildURL(path.Join("clients", clientID))
	if err != nil {
		return err
	}

	// Add reason as query parameter if provided
	if reason != "" {
		query := fullURL.Query()
		query.Set("reason", reason)
		fullURL.RawQuery = query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, "DELETE", fullURL.String(), nil)
	if err != nil {
		return err
	}

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
