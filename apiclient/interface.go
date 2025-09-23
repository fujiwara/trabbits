package apiclient

import (
	"context"

	"github.com/fujiwara/trabbits/config"
	"github.com/fujiwara/trabbits/types"
)

// APIClient interface defines the API client contract
type APIClient interface {
	// Config operations
	GetConfig(ctx context.Context) (*config.Config, error)
	PutConfigFromFile(ctx context.Context, configPath string) error
	PutConfig(ctx context.Context, cfg *config.Config) error
	DiffConfigFromFile(ctx context.Context, configPath string) (string, error)
	ReloadConfig(ctx context.Context) (*config.Config, error)

	// Client operations
	GetClients(ctx context.Context) ([]types.ClientInfo, error)
	GetClientDetail(ctx context.Context, clientID string) (*types.FullClientInfo, error)
	ShutdownClient(ctx context.Context, clientID, reason string) error

	// Probe operations
	StreamProbeLog(ctx context.Context, proxyID, format string) error
	StreamProbeLogEntries(ctx context.Context, proxyID string) (<-chan types.ProbeLogEntry, error)
}

// Ensure that Client implements APIClient
var _ APIClient = (*Client)(nil)
