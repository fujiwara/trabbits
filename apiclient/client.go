package apiclient

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"time"
)

const (
	// DefaultTimeout is the default timeout for regular API requests
	DefaultTimeout = 30 * time.Second
)

// Client represents a client for communicating with trabbits API server via Unix socket
type Client struct {
	endpoint     string
	client       *http.Client
	streamClient *http.Client // HTTP client without timeout for streaming operations
}

// New creates a new API client that communicates via Unix socket
func New(socketPath string) *Client {
	var transport http.RoundTripper
	var endpoint string

	// If socketPath starts with "http://" or "https://", treat it as an HTTP endpoint (for testing)
	if u, err := url.Parse(socketPath); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		endpoint = socketPath
		transport = http.DefaultTransport
	} else {
		// Otherwise, treat it as a Unix socket path
		endpoint = "http://localhost/"
		transport = &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		}
	}

	return &Client{
		endpoint: endpoint,
		client: &http.Client{
			Transport: transport,
			Timeout:   DefaultTimeout,
		},
		streamClient: &http.Client{
			Transport: transport,
			// No timeout for streaming operations
		},
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
