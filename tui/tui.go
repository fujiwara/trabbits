package tui

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fujiwara/trabbits/apiclient"
)

// Run starts the TUI application
func Run(ctx context.Context, apiClient apiclient.APIClient) error {
	model := NewModel(ctx, apiClient)

	// Start streaming server logs from API
	go func() {
		logChan, err := apiClient.StreamServerLogs(ctx)
		if err != nil {
			slog.Error("Failed to start server log stream", "error", err)
			return
		}

		// Forward server logs to TUI log channel
		for log := range logChan {
			// Extract level from attrs if present
			level := "INFO"
			if log.Attrs != nil {
				if l, ok := log.Attrs["level"].(string); ok {
					level = l
				}
			}

			// Convert ProbeLogEntry to LogEntry
			select {
			case model.GetLogChannel() <- LogEntry{
				Time:    log.Timestamp,
				Level:   level,
				Message: log.Message,
				Attrs:   log.Attrs,
			}:
			case <-ctx.Done():
				return
			}
		}
	}()

	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// handleKeyPress handles keyboard input based on current view mode
func (m *TUIModel) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global keys that work in all views
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		if m.viewMode == ViewList {
			return m, tea.Quit
		}
	}

	// View-specific key handling
	switch m.viewMode {
	case ViewList:
		return m.handleListKeys(msg)
	case ViewDetail:
		return m.handleDetailKeys(msg)
	case ViewConfirm:
		return m.handleConfirmKeys(msg)
	case ViewProbe:
		return m.handleProbeKeys(msg)
	case ViewServerLogs:
		return m.handleServerLogsKeys(msg)
	}

	return m, nil
}

// handleListKeys handles keys in the list view
func (m *TUIModel) handleListKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "K":
		// Shutdown client
		if len(m.clients) > 0 && m.selectedIdx < len(m.clients) {
			client := m.clients[m.selectedIdx]
			if client.ID == "" {
				m.err = fmt.Errorf("selected client has empty ID (index: %d, total: %d)", m.selectedIdx, len(m.clients))
				m.errorTime = time.Now()
				return m, nil
			}
			m.confirmState = &confirmState{
				clientID: client.ID,
				message:  fmt.Sprintf("Shutdown client %s (%s@%s)?", formatID(client.ID), client.User, client.ClientAddress),
			}
			m.viewMode = ViewConfirm
		}
	case "up", "k":
		if m.selectedIdx > 0 {
			m.selectedIdx--
			m.adjustScrollForSelection()
		}
	case "down", "j":
		if m.selectedIdx < len(m.clients)-1 {
			m.selectedIdx++
			m.adjustScrollForSelection()
		}
	case "pgup":
		pageSize := m.getVisibleRows()
		if m.selectedIdx > pageSize {
			m.selectedIdx -= pageSize
		} else {
			m.selectedIdx = 0
		}
		m.adjustScrollForSelection()
	case "pgdn":
		pageSize := m.getVisibleRows()
		if m.selectedIdx+pageSize < len(m.clients) {
			m.selectedIdx += pageSize
		} else {
			m.selectedIdx = len(m.clients) - 1
		}
		m.adjustScrollForSelection()
	case "home":
		m.selectedIdx = 0
		m.listScroll = 0
	case "end":
		m.selectedIdx = len(m.clients) - 1
		m.adjustScrollForSelection()
	case "enter":
		if len(m.clients) > 0 {
			m.selectedID = m.clients[m.selectedIdx].ID
			return m, m.fetchClientDetail(m.selectedID)
		}
	case "r":
		return m, m.fetchClients()
	case "p":
		// Start probe stream for selected client
		if len(m.clients) > 0 {
			clientID := m.clients[m.selectedIdx].ID
			m.viewMode = ViewProbe
			return m, m.startProbeStream(clientID)
		}
	case "l":
		// Switch to server logs view
		m.viewMode = ViewServerLogs
		// Initialize scroll to bottom
		if len(m.logEntries) > 0 {
			m.serverLogsSelectedIdx = len(m.logEntries) - 1
		}
	}
	return m, nil
}

// handleDetailKeys handles keys in the detail view
func (m *TUIModel) handleDetailKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		m.viewMode = ViewList
		m.clientDetail = nil
		m.detailScroll = 0
	case "K":
		// Shutdown client from detail view
		if m.clientDetail != nil {
			m.confirmState = &confirmState{
				clientID: m.clientDetail.ID,
				message:  fmt.Sprintf("Shutdown client %s (%s@%s)?", formatID(m.clientDetail.ID), m.clientDetail.User, m.clientDetail.ClientAddress),
			}
			m.viewMode = ViewConfirm
		}
	case "up", "k":
		if m.detailScroll > 0 {
			m.detailScroll--
		}
	case "down", "j":
		// Allow scrolling down (we'll implement limits in renderDetailView)
		m.detailScroll++
	case "home":
		m.detailScroll = 0
	case "end":
		// Set to large number, will be limited in render
		m.detailScroll = 1000
	case "p":
		// Start probe stream for current client
		if m.clientDetail != nil {
			m.viewMode = ViewProbe
			return m, m.startProbeStream(m.clientDetail.ID)
		}
	}
	return m, nil
}

// handleConfirmKeys handles keys in the confirmation dialog
func (m *TUIModel) handleConfirmKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q", "esc", "n":
		m.viewMode = ViewList
		m.confirmState = nil
	case "y", "enter":
		if m.confirmState != nil {
			clientID := m.confirmState.clientID
			m.viewMode = ViewList
			m.confirmState = nil
			return m, m.shutdownClient(clientID)
		}
	}
	return m, nil
}

// handleProbeKeys handles keys in the probe view
func (m *TUIModel) handleProbeKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		// Stop probe stream and return to list view
		m.stopProbeStream()
		m.viewMode = ViewList
	case "up", "k":
		if m.probeState != nil && len(m.probeState.logs) > 0 {
			if m.probeState.selectedIdx > 0 {
				m.probeState.selectedIdx--
				// Adjust scroll if selected item is above visible area
				if m.probeState.selectedIdx < m.probeState.scroll {
					m.probeState.scroll = m.probeState.selectedIdx
				}
			}
			m.probeState.autoScroll = false // Disable auto-scroll on manual navigation
		}
	case "down", "j":
		if m.probeState != nil && len(m.probeState.logs) > 0 {
			logCount := len(m.probeState.logs)
			if m.probeState.selectedIdx < logCount-1 {
				m.probeState.selectedIdx++
				// Adjust scroll if selected item is below visible area
				visibleRows := m.getProbeVisibleRows()
				if m.probeState.selectedIdx >= m.probeState.scroll+visibleRows {
					m.probeState.scroll = m.probeState.selectedIdx - visibleRows + 1
				}
			}
			m.updateAutoScroll() // Check if we should enable/disable auto-scroll
		}
	case "home":
		if m.probeState != nil {
			m.probeState.scroll = 0
			m.probeState.selectedIdx = 0
			m.probeState.autoScroll = false // Disable auto-scroll when going to top
		}
	case "end":
		if m.probeState != nil && len(m.probeState.logs) > 0 {
			m.probeState.selectedIdx = len(m.probeState.logs) - 1
			// Adjust scroll to show the last item
			visibleRows := m.getProbeVisibleRows()
			maxScroll := len(m.probeState.logs) - visibleRows
			if maxScroll < 0 {
				maxScroll = 0
			}
			m.probeState.scroll = maxScroll
			m.probeState.autoScroll = true // Enable auto-scroll when going to bottom
		}
	case "pgup":
		if m.probeState != nil && len(m.probeState.logs) > 0 {
			pageSize := m.getProbeVisibleRows()
			if m.probeState.selectedIdx > pageSize {
				m.probeState.selectedIdx -= pageSize
			} else {
				m.probeState.selectedIdx = 0
			}
			// Adjust scroll
			if m.probeState.selectedIdx < m.probeState.scroll {
				m.probeState.scroll = m.probeState.selectedIdx
			}
			m.probeState.autoScroll = false // Disable auto-scroll when paging up
		}
	case "pgdn":
		if m.probeState != nil && len(m.probeState.logs) > 0 {
			logCount := len(m.probeState.logs)
			pageSize := m.getProbeVisibleRows()
			if m.probeState.selectedIdx+pageSize < logCount-1 {
				m.probeState.selectedIdx += pageSize
			} else {
				m.probeState.selectedIdx = logCount - 1
			}
			// Adjust scroll
			visibleRows := m.getProbeVisibleRows()
			if m.probeState.selectedIdx >= m.probeState.scroll+visibleRows {
				m.probeState.scroll = m.probeState.selectedIdx - visibleRows + 1
			}
			m.updateAutoScroll() // Check if we should enable/disable auto-scroll
		}
	}
	return m, nil
}

// updateAutoScroll checks if we're at the bottom and enables/disables auto-scroll accordingly
func (m *TUIModel) updateAutoScroll() {
	if m.probeState == nil {
		return
	}

	logCount := len(m.probeState.logs)
	visibleRows := m.getProbeVisibleRows()
	maxScroll := logCount - visibleRows
	if maxScroll < 0 {
		maxScroll = 0
	}

	// Enable auto-scroll only when we're at or near the bottom
	m.probeState.autoScroll = m.probeState.scroll >= maxScroll
}

// getProbeVisibleRows calculates visible rows for probe logs
func (m *TUIModel) getProbeVisibleRows() int {
	// Reserve space for header, footer, and status
	return m.height - 6
}

// handleServerLogsKeys handles keys in the server logs view
func (m *TUIModel) handleServerLogsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		m.viewMode = ViewList
		m.serverLogsScroll = 0
	case "up", "k":
		if len(m.logEntries) > 0 && m.serverLogsSelectedIdx > 0 {
			m.serverLogsSelectedIdx--
			m.adjustServerLogsScroll()
		}
	case "down", "j":
		if len(m.logEntries) > 0 && m.serverLogsSelectedIdx < len(m.logEntries)-1 {
			m.serverLogsSelectedIdx++
			m.adjustServerLogsScroll()
		}
	case "pgup":
		visibleRows := m.getServerLogsVisibleRows()
		if m.serverLogsSelectedIdx > visibleRows {
			m.serverLogsSelectedIdx -= visibleRows
		} else {
			m.serverLogsSelectedIdx = 0
		}
		m.adjustServerLogsScroll()
	case "pgdn":
		visibleRows := m.getServerLogsVisibleRows()
		if m.serverLogsSelectedIdx+visibleRows < len(m.logEntries)-1 {
			m.serverLogsSelectedIdx += visibleRows
		} else {
			m.serverLogsSelectedIdx = len(m.logEntries) - 1
		}
		m.adjustServerLogsScroll()
	case "home":
		m.serverLogsSelectedIdx = 0
		m.serverLogsScroll = 0
	case "end":
		if len(m.logEntries) > 0 {
			m.serverLogsSelectedIdx = len(m.logEntries) - 1
			m.adjustServerLogsScroll()
		}
	}
	return m, nil
}

// getServerLogsVisibleRows calculates visible rows for server logs view
func (m *TUIModel) getServerLogsVisibleRows() int {
	// Reserve space for header, footer, and status
	return m.height - 6
}

// adjustServerLogsScroll adjusts scroll position to keep selected log visible
func (m *TUIModel) adjustServerLogsScroll() {
	visibleRows := m.getServerLogsVisibleRows()

	// If selected item is above visible area, scroll up
	if m.serverLogsSelectedIdx < m.serverLogsScroll {
		m.serverLogsScroll = m.serverLogsSelectedIdx
	}

	// If selected item is below visible area, scroll down
	if m.serverLogsSelectedIdx >= m.serverLogsScroll+visibleRows {
		m.serverLogsScroll = m.serverLogsSelectedIdx - visibleRows + 1
	}

	// Ensure scroll is within bounds
	if m.serverLogsScroll < 0 {
		m.serverLogsScroll = 0
	}
	maxScroll := len(m.logEntries) - visibleRows
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.serverLogsScroll > maxScroll {
		m.serverLogsScroll = maxScroll
	}
}

// RunTUI starts the TUI with the provided socket path
func RunTUI(ctx context.Context, socketPath string) error {
	// We'll call the main package to create the API client
	// This will be implemented when we update the CLI
	return fmt.Errorf("TUI not yet integrated - use Run() with APIClient interface")
}
