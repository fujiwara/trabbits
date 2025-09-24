package apiclient

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Client represents a client for communicating with trabbits API server via Unix socket
type Client struct {
	endpoint string
	client   *http.Client
}

// New creates a new API client that communicates via Unix socket
func New(socketPath string) *Client {
	// If socketPath starts with "http://" or "https://", treat it as an HTTP endpoint (for testing)
	if u, err := url.Parse(socketPath); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		return &Client{
			endpoint: socketPath,
			client: &http.Client{
				Timeout: 30 * time.Second,
			},
		}
	}

	// Otherwise, treat it as a Unix socket path
	tr := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   30 * time.Second,
	}
	return &Client{
		endpoint: "http://localhost/",
		client:   client,
	}
}

// buildURL constructs a full URL from the endpoint and path
func (c *Client) buildURL(pathStr string) (*url.URL, error) {
	baseURL, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, err
	}

	u, err := url.Parse(pathStr)
	if err != nil {
		return nil, err
	}

	return baseURL.ResolveReference(u), nil
}
