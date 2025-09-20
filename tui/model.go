package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fujiwara/trabbits/types"
)

// ViewMode represents the current view state
type ViewMode int

const (
	ViewList ViewMode = iota
	ViewDetail
	ViewConfirm
)

// TUIModel represents the TUI application state
type TUIModel struct {
	ctx          context.Context
	apiClient    APIClient
	clients      []types.ClientInfo
	selectedIdx  int
	viewMode     ViewMode
	selectedID   string
	clientDetail *types.FullClientInfo
	confirmState *confirmState
	width        int
	height       int
	lastUpdate   time.Time
	err          error
	errorTime    time.Time
	successMsg   string
	successTime  time.Time
	detailScroll int
	listScroll   int
}

type confirmState struct {
	clientID string
	message  string
}

// APIClient interface for TUI to interact with the trabbits API
type APIClient interface {
	GetClients(ctx context.Context) ([]types.ClientInfo, error)
	GetClientDetail(ctx context.Context, clientID string) (*types.FullClientInfo, error)
	ShutdownClient(ctx context.Context, clientID, reason string) error
}

// Message types for Bubble Tea
type (
	tickMsg         struct{}
	clientsMsg      []types.ClientInfo
	clientDetailMsg *types.FullClientInfo
	errorMsg        error
	successMsg      string
)

// NewModel creates a new TUI model
func NewModel(ctx context.Context, apiClient APIClient) *TUIModel {
	return &TUIModel{
		ctx:       ctx,
		apiClient: apiClient,
		clients:   []types.ClientInfo{},
		viewMode:  ViewList,
	}
}

// Init initializes the TUI model
func (m *TUIModel) Init() tea.Cmd {
	return tea.Batch(
		m.fetchClients(),
		tick(),
	)
}

// Update handles TUI messages and state updates
func (m *TUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tickMsg:
		if time.Since(m.lastUpdate) > 2*time.Second {
			// Always fetch clients list
			cmds := []tea.Cmd{m.fetchClients(), tick()}

			// If in detail view, also fetch updated client detail
			if m.viewMode == ViewDetail && m.clientDetail != nil {
				cmds = append(cmds, m.fetchClientDetail(m.clientDetail.ID))
			}

			return m, tea.Batch(cmds...)
		}
		return m, tick()

	case clientsMsg:
		m.clients = []types.ClientInfo(msg)
		m.lastUpdate = time.Now()
		if m.selectedIdx >= len(m.clients) && len(m.clients) > 0 {
			m.selectedIdx = len(m.clients) - 1
		}
		return m, nil

	case clientDetailMsg:
		m.clientDetail = (*types.FullClientInfo)(msg)
		m.viewMode = ViewDetail
		return m, nil

	case errorMsg:
		m.err = error(msg)
		m.errorTime = time.Now()
		return m, nil

	case successMsg:
		m.successMsg = string(msg)
		m.successTime = time.Now()
		return m, nil
	}

	return m, nil
}

// fetchClients fetches the client list from the API
func (m *TUIModel) fetchClients() tea.Cmd {
	return func() tea.Msg {
		clients, err := m.apiClient.GetClients(m.ctx)
		if err != nil {
			return errorMsg(err)
		}
		return clientsMsg(clients)
	}
}

// fetchClientDetail fetches detailed client information
func (m *TUIModel) fetchClientDetail(clientID string) tea.Cmd {
	return func() tea.Msg {
		clientInfo, err := m.apiClient.GetClientDetail(m.ctx, clientID)
		if err != nil {
			return errorMsg(err)
		}
		return clientDetailMsg(clientInfo)
	}
}

// shutdownClient initiates client shutdown
func (m *TUIModel) shutdownClient(clientID string) tea.Cmd {
	return func() tea.Msg {
		err := m.apiClient.ShutdownClient(m.ctx, clientID, "TUI shutdown")
		if err != nil {
			return errorMsg(err)
		}
		return successMsg("Client shutdown initiated successfully")
	}
}

// tick creates a tick message for periodic updates
func tick() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}