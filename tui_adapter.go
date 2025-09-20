package trabbits

import (
	"context"

	"github.com/fujiwara/trabbits/tui"
	"github.com/fujiwara/trabbits/types"
)

// apiClientAdapter adapts the existing apiClient to the TUI APIClient interface
type apiClientAdapter struct {
	client *apiClient
}

// newAPIClientAdapter creates a new API client adapter for TUI
func newAPIClientAdapter(client *apiClient) tui.APIClient {
	return &apiClientAdapter{
		client: client,
	}
}

// GetClients implements tui.APIClient
func (a *apiClientAdapter) GetClients(ctx context.Context) ([]types.ClientInfo, error) {
	return a.client.getClients(ctx)
}

// GetClientDetail implements tui.APIClient
func (a *apiClientAdapter) GetClientDetail(ctx context.Context, clientID string) (*types.FullClientInfo, error) {
	return a.client.getClientDetail(ctx, clientID)
}

// ShutdownClient implements tui.APIClient
func (a *apiClientAdapter) ShutdownClient(ctx context.Context, clientID, reason string) error {
	return a.client.shutdownClient(ctx, clientID, reason)
}

// runTUI starts the TUI using the new tui package
func runTUI(ctx context.Context, opt *CLI) error {
	client := newAPIClient(opt.APISocket)
	adapter := newAPIClientAdapter(client)
	return tui.Run(ctx, adapter)
}