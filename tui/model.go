package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fujiwara/trabbits/apiclient"
	"github.com/fujiwara/trabbits/types"
)

// ViewMode represents the current view state
type ViewMode int

const (
	ViewList ViewMode = iota
	ViewDetail
	ViewConfirm
	ViewProbe
)

// TUIModel represents the TUI application state
type TUIModel struct {
	ctx          context.Context
	apiClient    apiclient.APIClient
	clients      []types.ClientInfo
	selectedIdx  int
	viewMode     ViewMode
	selectedID   string
	clientDetail *types.FullClientInfo
	confirmState *confirmState
	probeState   *probeState
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

type probeState struct {
	clientID   string
	logs       []probeLogEntry
	scroll     int
	cancelFunc context.CancelFunc
	ctx        context.Context
	logChan    <-chan ProbeLogEntry
	autoScroll bool // whether to auto-scroll to latest logs
}

// Use types.ProbeLogEntry instead of local definition
type ProbeLogEntry = types.ProbeLogEntry

type probeLogEntry = ProbeLogEntry

// APIClient is aliased from apiclient.APIClient
type APIClient = apiclient.APIClient

// Message types for Bubble Tea
type (
	tickMsg               struct{}
	clientsMsg            []types.ClientInfo
	clientDetailMsg       *types.FullClientInfo
	errorMsg              error
	successMsg            string
	probeLogMsg           probeLogEntry
	probeEndMsg           struct{}
	probeStreamStartedMsg struct {
		clientID   string
		ctx        context.Context
		logChan    <-chan types.ProbeLogEntry
		cancelFunc context.CancelFunc
	}
)

// NewModel creates a new TUI model
func NewModel(ctx context.Context, apiClient apiclient.APIClient) *TUIModel {
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

	case probeLogMsg:
		if m.probeState != nil {
			entry := probeLogEntry(msg)
			m.probeState.logs = append(m.probeState.logs, entry)
			// Keep only last 1000 logs to prevent memory issues
			if len(m.probeState.logs) > 1000 {
				m.probeState.logs = m.probeState.logs[len(m.probeState.logs)-1000:]
			}
			// Auto-scroll to bottom only if auto-scroll is enabled
			if m.probeState.autoScroll {
				m.probeState.scroll = len(m.probeState.logs)
			}

			// Continue listening for next log
			return m, m.listenForProbeLog()
		}
		return m, nil

	case probeEndMsg:
		if m.probeState != nil {
			m.err = fmt.Errorf("probe stream ended for client %s", m.probeState.clientID)
			m.errorTime = time.Now()
		}
		return m, nil

	case probeStreamStartedMsg:
		// Initialize probe state and start listening
		m.probeState = &probeState{
			clientID:   msg.clientID,
			logs:       []probeLogEntry{},
			scroll:     0,
			cancelFunc: msg.cancelFunc,
			ctx:        msg.ctx,
			logChan:    msg.logChan,
			autoScroll: true, // start with auto-scroll enabled
		}
		return m, m.listenForProbeLog()
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

// startProbeStream starts probe log streaming for a client
func (m *TUIModel) startProbeStream(clientID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(m.ctx)

		logChan, err := m.apiClient.StreamProbeLogEntries(ctx, clientID)
		if err != nil {
			cancel()
			return errorMsg(err)
		}

		// Return a message that will set up the probe state
		return probeStreamStartedMsg{
			clientID:   clientID,
			ctx:        ctx,
			logChan:    logChan,
			cancelFunc: cancel,
		}
	}
}

// listenForProbeLog creates a command to listen for the next probe log
func (m *TUIModel) listenForProbeLog() tea.Cmd {
	return func() tea.Msg {
		if m.probeState == nil {
			return probeEndMsg{}
		}

		select {
		case <-m.probeState.ctx.Done():
			return probeEndMsg{}
		case log, ok := <-m.probeState.logChan:
			if !ok {
				return probeEndMsg{}
			}
			return probeLogMsg(log)
		}
	}
}

// stopProbeStream stops the current probe stream
func (m *TUIModel) stopProbeStream() {
	if m.probeState != nil && m.probeState.cancelFunc != nil {
		m.probeState.cancelFunc()
		m.probeState = nil
	}
}

// tick creates a tick message for periodic updates
func tick() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}
