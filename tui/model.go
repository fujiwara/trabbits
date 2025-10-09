package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	ViewServerLogs
	ViewSaveConfirm
)

// TUIModel represents the TUI application state
type TUIModel struct {
	ctx                   context.Context
	apiClient             apiclient.APIClient
	clients               []types.ClientInfo
	selectedIdx           int
	viewMode              ViewMode
	selectedID            string
	clientDetail          *types.FullClientInfo
	confirmState          *confirmState
	probeState            *probeState
	saveState             *saveState
	width                 int
	height                int
	lastUpdate            time.Time
	err                   error
	errorTime             time.Time
	successMsg            string
	successTime           time.Time
	detailScroll          int
	listScroll            int
	logEntries            []LogEntry // Recent log messages from slog
	logChan               chan LogEntry
	serverLogsScroll      int   // Scroll position for server logs view
	serverLogsSelectedIdx int   // Selected log index in server logs view
	droppedLogs           int64 // Number of dropped server log entries (non-blocking send)
}

// LogEntry represents a log message
type LogEntry struct {
	Time     time.Time
	Level    string
	Message  string
	Attrs    map[string]any
	AttrJSON string // cached JSON string for Attrs (filtered)
}

type confirmState struct {
	clientID string
	message  string
}

type saveState struct {
	clientID         string
	filePath         string
	editing          bool
	cursorPos        int
	previousView     ViewMode
	overwriteConfirm bool
}

type probeState struct {
	clientID       string
	logs           []probeLogEntry
	scroll         int
	selectedIdx    int // Currently selected log index
	cancelFunc     context.CancelFunc
	ctx            context.Context
	logChan        <-chan ProbeLogEntry
	autoScroll     bool // whether to auto-scroll to latest logs
	reconnectCount int  // Track reconnection attempts
	disconnected   bool // true if this is a disconnected proxy (no reconnection)
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
	logMsg                LogEntry
	probeStreamStartedMsg struct {
		clientID   string
		ctx        context.Context
		logChan    <-chan types.ProbeLogEntry
		cancelFunc context.CancelFunc
	}
	reconnectProbeMsg struct {
		clientID       string
		logs           []probeLogEntry
		reconnectCount int
	}
	probeStreamStartedMsgWithState struct {
		clientID       string
		ctx            context.Context
		logChan        <-chan types.ProbeLogEntry
		cancelFunc     context.CancelFunc
		preservedLogs  []probeLogEntry
		reconnectCount int
		disconnected   bool
	}
)

// NewModel creates a new TUI model
func NewModel(ctx context.Context, apiClient apiclient.APIClient) *TUIModel {
	logChan := make(chan LogEntry, 100)
	return &TUIModel{
		ctx:        ctx,
		apiClient:  apiClient,
		clients:    []types.ClientInfo{},
		viewMode:   ViewList,
		logEntries: []LogEntry{},
		logChan:    logChan,
	}
}

// GetLogChannel returns the log channel for external log writers
func (m *TUIModel) GetLogChannel() chan<- LogEntry {
	return m.logChan
}

// Init initializes the TUI model
func (m *TUIModel) Init() tea.Cmd {
	return tea.Batch(
		m.fetchClients(),
		tick(),
		m.listenForLogs(),
	)
}

// Update handles TUI messages and state updates
func (m *TUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Re-clamp scroll/selection for the active view after resize
		switch m.viewMode {
		case ViewList:
			m.adjustScrollForSelection()
		case ViewServerLogs:
			visible := m.getServerLogsVisibleRows()
			total := len(m.logEntries)
			total = clamp(total, 0, total) // no-op, placeholder for clarity
			m.serverLogsSelectedIdx = clamp(m.serverLogsSelectedIdx, 0, clamp(total-1, -1, total-1))
			m.serverLogsScroll = clampScrollToContain(m.serverLogsScroll, m.serverLogsSelectedIdx, visible, total)
			// Ensure bounds when there are no logs
			if total == 0 {
				m.serverLogsSelectedIdx = 0
				m.serverLogsScroll = 0
			}
		case ViewProbe:
			if m.probeState != nil {
				visible := m.getProbeVisibleRows()
				total := len(m.probeState.logs)
				m.probeState.selectedIdx = clamp(m.probeState.selectedIdx, 0, clamp(total-1, -1, total-1))
				m.probeState.scroll = clampScrollToContain(m.probeState.scroll, m.probeState.selectedIdx, visible, total)
			}
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tickMsg:
		// Always fetch clients list on every tick (every 2 seconds)
		cmds := []tea.Cmd{m.fetchClients(), tick()}

		// If in detail view, also fetch updated client detail
		if m.viewMode == ViewDetail && m.clientDetail != nil {
			cmds = append(cmds, m.fetchClientDetail(m.clientDetail.ID))
		}

		return m, tea.Batch(cmds...)

	case clientsMsg:
		// Preserve selection by client ID when possible
		var prevSelectedID string
		if m.selectedIdx >= 0 && m.selectedIdx < len(m.clients) {
			prevSelectedID = m.clients[m.selectedIdx].ID
		}
		m.clients = []types.ClientInfo(msg)

		// Sort clients: active first (oldest connection first), then closed (newest disconnection first)
		sort.SliceStable(m.clients, func(i, j int) bool {
			ci, cj := m.clients[i], m.clients[j]

			// Active clients come before closed clients
			if ci.Status == "active" && cj.Status != "active" {
				return true
			}
			if ci.Status != "active" && cj.Status == "active" {
				return false
			}

			// Among active clients, older connections first
			if ci.Status == "active" && cj.Status == "active" {
				return ci.ConnectedAt.Before(cj.ConnectedAt)
			}

			// Among closed clients, newer disconnections first (most recently closed)
			if ci.Status == "disconnected" && cj.Status == "disconnected" {
				return ci.DisconnectedAt.After(cj.DisconnectedAt)
			}

			// Default: maintain order
			return false
		})

		m.lastUpdate = time.Now()
		// Try to restore selection to the same client ID
		if prevSelectedID != "" {
			restored := false
			for i, c := range m.clients {
				if c.ID == prevSelectedID {
					m.selectedIdx = i
					restored = true
					break
				}
			}
			if !restored && len(m.clients) > 0 && m.selectedIdx >= len(m.clients) {
				m.selectedIdx = len(m.clients) - 1
			}
		} else if len(m.clients) > 0 && m.selectedIdx >= len(m.clients) {
			m.selectedIdx = len(m.clients) - 1
		}
		// Ensure selection remains visible and scroll stays in bounds
		m.adjustScrollForSelection()
		return m, nil

	case clientDetailMsg:
		m.clientDetail = (*types.FullClientInfo)(msg)
		m.viewMode = ViewDetail
		return m, nil

	case errorMsg:
		m.err = error(msg)
		m.errorTime = time.Now()
		// If we were trying to start probe stream and got an error, go back to list
		if m.viewMode == ViewProbe && m.probeState == nil {
			m.viewMode = ViewList
		}
		return m, nil

	case successMsg:
		m.successMsg = string(msg)
		m.successTime = time.Now()
		// If we were in save confirm view, return to previous view
		if m.viewMode == ViewSaveConfirm && m.saveState != nil {
			m.viewMode = m.saveState.previousView
			m.saveState = nil
		}
		return m, nil

	case probeLogMsg:
		if m.probeState != nil {
			entry := probeLogEntry(msg)
			m.probeState.logs = append(m.probeState.logs, entry)
			// Keep only last N logs to prevent memory issues
			if len(m.probeState.logs) > probeLogKeep {
				m.probeState.logs = m.probeState.logs[len(m.probeState.logs)-probeLogKeep:]
			}
			// Auto-scroll to bottom only if auto-scroll is enabled
			if m.probeState.autoScroll {
				m.probeState.selectedIdx = len(m.probeState.logs) - 1 // Select the latest log
				// Adjust scroll to show the last item
				visibleRows := m.getProbeVisibleRows()
				total := len(m.probeState.logs)
				m.probeState.scroll = maxScroll(total, visibleRows)
			}
			// When auto-scroll is disabled, still keep scroll within legal bounds
			if !m.probeState.autoScroll {
				visible := m.getProbeVisibleRows()
				total := len(m.probeState.logs)
				// Keep selectedIdx inside bounds
				m.probeState.selectedIdx = clamp(m.probeState.selectedIdx, 0, clamp(total-1, -1, total-1))
				m.probeState.scroll = clamp(m.probeState.scroll, 0, maxScroll(total, visible))
			}

			// Continue listening for next log
			return m, m.listenForProbeLog()
		}
		return m, nil

	case probeEndMsg:
		if m.probeState != nil {
			// If we're no longer in probe view, don't attempt reconnection
			if m.viewMode != ViewProbe {
				m.stopProbeStream()
				return m, nil
			}

			// If this is a disconnected proxy, don't attempt reconnection
			if m.probeState.disconnected {
				// Cancel the context but keep probeState to display logs
				if m.probeState.cancelFunc != nil {
					m.probeState.cancelFunc()
					m.probeState.cancelFunc = nil // Clear to prevent double-cancel
				}
				return m, nil
			}

			// Probe stream ended, attempt to reconnect
			clientID := m.probeState.clientID
			reconnectCount := m.probeState.reconnectCount

			// Limit reconnection attempts
			if reconnectCount >= 5 {
				m.err = fmt.Errorf("probe stream ended after %d reconnection attempts", reconnectCount)
				m.errorTime = time.Now()
				m.stopProbeStream()
				return m, nil
			}

			// Clean up old stream but preserve logs
			logs := m.probeState.logs
			m.stopProbeStream()

			// Add a message about reconnecting
			m.err = fmt.Errorf("probe stream disconnected, reconnecting... (attempt %d/5)", reconnectCount+1)
			m.errorTime = time.Now()

			// Restart the probe stream with incremented reconnect count after a short delay
			return m, tea.Tick(time.Second, func(time.Time) tea.Msg {
				return reconnectProbeMsg{clientID: clientID, logs: logs, reconnectCount: reconnectCount + 1}
			})
		}
		return m, nil

	case probeStreamStartedMsg:
		// Initialize probe state and start listening
		m.probeState = &probeState{
			clientID:    msg.clientID,
			logs:        []probeLogEntry{},
			scroll:      0,
			selectedIdx: 0,
			cancelFunc:  msg.cancelFunc,
			ctx:         msg.ctx,
			logChan:     msg.logChan,
			autoScroll:  true, // start with auto-scroll enabled
		}
		return m, m.listenForProbeLog()

	case reconnectProbeMsg:
		// Handle reconnection with preserved logs only when still in probe view
		if m.viewMode != ViewProbe {
			return m, nil
		}
		return m, m.startProbeStreamWithState(msg.clientID, msg.logs, msg.reconnectCount, false)

	case probeStreamStartedMsgWithState:
		// Initialize probe state with preserved logs and start listening
		if m.viewMode != ViewProbe {
			// View changed while starting; cancel to avoid leak
			if msg.cancelFunc != nil {
				msg.cancelFunc()
			}
			return m, nil
		}
		logs := msg.preservedLogs
		if logs == nil {
			logs = []probeLogEntry{}
		}
		m.probeState = &probeState{
			clientID:       msg.clientID,
			logs:           logs,
			scroll:         0,
			selectedIdx:    len(logs) - 1, // Select the last log if there are existing logs
			cancelFunc:     msg.cancelFunc,
			ctx:            msg.ctx,
			logChan:        msg.logChan,
			autoScroll:     true,
			reconnectCount: msg.reconnectCount,
			disconnected:   msg.disconnected,
		}
		// Ensure selectedIdx is not negative
		if m.probeState.selectedIdx < 0 {
			m.probeState.selectedIdx = 0
		}
		// If we already have logs, set initial scroll so the last page is visible
		if len(logs) > 0 {
			visible := m.getProbeVisibleRows()
			if visible < 1 {
				visible = 1
			}
			maxScroll := len(logs) - visible
			if maxScroll < 0 {
				maxScroll = 0
			}
			m.probeState.scroll = maxScroll
		}
		return m, m.listenForProbeLog()

	case logMsg:
		// Add log entry to buffer with cached AttrJSON
		entry := LogEntry(msg)
		if len(entry.Attrs) > 0 {
			// Filter out level before caching
			filtered := make(map[string]any)
			for k, v := range entry.Attrs {
				if k != "level" {
					filtered[k] = v
				}
			}
			if len(filtered) > 0 {
				if b, err := json.Marshal(filtered); err == nil {
					entry.AttrJSON = string(b)
				}
			}
		}
		m.logEntries = append(m.logEntries, entry)
		if len(m.logEntries) > serverLogKeep {
			m.logEntries = m.logEntries[len(m.logEntries)-serverLogKeep:]
		}
		// If we are in the server logs view, keep selection/scroll within bounds after update
		if m.viewMode == ViewServerLogs {
			visible := m.getServerLogsVisibleRows()
			total := len(m.logEntries)
			m.serverLogsSelectedIdx = clamp(m.serverLogsSelectedIdx, 0, clamp(total-1, -1, total-1))
			m.serverLogsScroll = clampScrollToContain(m.serverLogsScroll, m.serverLogsSelectedIdx, visible, total)
		}
		// Continue listening for logs
		return m, m.listenForLogs()
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
	return m.startProbeStreamWithState(clientID, nil, 0, false)
}

// startProbeStreamWithDisconnected starts probe log streaming with disconnected flag
func (m *TUIModel) startProbeStreamWithDisconnected(clientID string, disconnected bool) tea.Cmd {
	return m.startProbeStreamWithState(clientID, nil, 0, disconnected)
}

// startProbeStreamWithState starts probe log streaming with preserved state
func (m *TUIModel) startProbeStreamWithState(clientID string, preservedLogs []probeLogEntry, reconnectCount int, disconnected bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(m.ctx)

		logChan, err := m.apiClient.StreamProbeLogEntries(ctx, clientID)
		if err != nil {
			cancel()
			return errorMsg(err)
		}

		// Set up a modified probeStreamStartedMsg to preserve logs and reconnect count
		return probeStreamStartedMsgWithState{
			clientID:       clientID,
			ctx:            ctx,
			logChan:        logChan,
			cancelFunc:     cancel,
			preservedLogs:  preservedLogs,
			reconnectCount: reconnectCount,
			disconnected:   disconnected,
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

// listenForLogs creates a command to listen for log entries
func (m *TUIModel) listenForLogs() tea.Cmd {
	return func() tea.Msg {
		select {
		case <-m.ctx.Done():
			return nil
		case log, ok := <-m.logChan:
			if !ok {
				return nil
			}
			return logMsg(log)
		}
	}
}

// tick creates a tick message for periodic updates
func tick() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

// saveProbeLogsToFile saves probe logs to the specified file
func (m *TUIModel) saveProbeLogsToFile() tea.Cmd {
	return func() tea.Msg {
		if m.saveState == nil || m.probeState == nil {
			return errorMsg(fmt.Errorf("save state or probe state not initialized"))
		}

		filePath := m.saveState.filePath
		logs := m.probeState.logs

		// Ensure parent directory exists if specified
		if dir := filepath.Dir(filePath); dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return errorMsg(fmt.Errorf("failed to create directory %s: %w", dir, err))
			}
		}

		// Create file
		file, err := os.Create(filePath)
		if err != nil {
			return errorMsg(fmt.Errorf("failed to create file: %w", err))
		}
		defer file.Close()

		// Write logs as JSON lines
		encoder := json.NewEncoder(file)
		for _, log := range logs {
			if err := encoder.Encode(log); err != nil {
				return errorMsg(fmt.Errorf("failed to write log: %w", err))
			}
		}

		return successMsg(fmt.Sprintf("Saved %d logs to %s", len(logs), filePath))
	}
}
