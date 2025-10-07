package tui

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fujiwara/trabbits/apiclient"
)

// Run starts the TUI application
func Run(ctx context.Context, apiClient apiclient.APIClient) error {
	// Ensure all background operations stop when TUI exits
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

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
			entry := LogEntry{
				Time:    log.Timestamp,
				Level:   level,
				Message: log.Message,
				Attrs:   log.Attrs,
			}

			// Non-blocking send to avoid UI stalls; drop if channel is full
			select {
			case <-ctx.Done():
				return
			default:
			}
			select {
			case model.GetLogChannel() <- entry:
			case <-ctx.Done():
				return
			default:
				// drop entry to avoid blocking; count it
				atomic.AddInt64(&model.droppedLogs, 1)
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
	case ViewSaveConfirm:
		return m.handleSaveConfirmKeys(msg)
	}

	return m, nil
}

// handleListKeys handles keys in the list view
func (m *TUIModel) handleListKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "g":
		// Jump to top (Vim-like)
		m.selectedIdx = 0
		m.listScroll = 0
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
	case "G":
		// Jump to bottom (Vim-like)
		if len(m.clients) > 0 {
			m.selectedIdx = len(m.clients) - 1
			m.adjustScrollForSelection()
		}
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
			// Scroll so that the last entry is visible
			visible := m.getServerLogsVisibleRows()
			if visible < 1 {
				visible = 1
			}
			maxScroll := len(m.logEntries) - visible
			if maxScroll < 0 {
				maxScroll = 0
			}
			m.serverLogsScroll = maxScroll
		} else {
			m.serverLogsSelectedIdx = 0
			m.serverLogsScroll = 0
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
	case "g":
		if m.probeState != nil {
			m.probeState.selectedIdx = 0
			m.probeState.scroll = 0
			m.probeState.autoScroll = false
		}
	case " ":
		// Toggle auto-scroll with space key
		if m.probeState != nil {
			m.probeState.autoScroll = !m.probeState.autoScroll
			// If enabling auto-scroll, jump to bottom
			if m.probeState.autoScroll && len(m.probeState.logs) > 0 {
				m.probeState.selectedIdx = len(m.probeState.logs) - 1
				visibleRows := m.getProbeVisibleRows()
				maxScroll := len(m.probeState.logs) - visibleRows
				if maxScroll < 0 {
					maxScroll = 0
				}
				m.probeState.scroll = maxScroll
			}
			// Show a short toast indicating the state
			if m.probeState.autoScroll {
				m.successMsg = "Auto-scroll: ON"
			} else {
				m.successMsg = "Auto-scroll: OFF"
			}
			m.successTime = time.Now()
		}
	case "s":
		// Save probe logs to file
		if m.probeState != nil {
			clientID := m.probeState.clientID
			timestamp := time.Now().Format("20060102-150405")
			defaultPath := fmt.Sprintf("%s-%s.log", clientID, timestamp)
			m.saveState = &saveState{
				clientID:     clientID,
				filePath:     defaultPath,
				editing:      false,
				cursorPos:    len(defaultPath),
				previousView: ViewProbe,
			}
			m.viewMode = ViewSaveConfirm
		}
	case "up", "k":
		if m.probeState != nil && len(m.probeState.logs) > 0 {
			total := len(m.probeState.logs)
			if m.probeState.selectedIdx > 0 {
				m.probeState.selectedIdx--
			}
			// Clamp scroll to contain the selection
			m.probeState.scroll = clampScrollToContain(m.probeState.scroll, m.probeState.selectedIdx, m.getProbeVisibleRows(), total)
			m.probeState.autoScroll = false // Disable auto-scroll on manual navigation
		}
	case "down", "j":
		if m.probeState != nil && len(m.probeState.logs) > 0 {
			total := len(m.probeState.logs)
			if m.probeState.selectedIdx < total-1 {
				m.probeState.selectedIdx++
			}
			m.probeState.scroll = clampScrollToContain(m.probeState.scroll, m.probeState.selectedIdx, m.getProbeVisibleRows(), total)
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
	case "G":
		if m.probeState != nil && len(m.probeState.logs) > 0 {
			m.probeState.selectedIdx = len(m.probeState.logs) - 1
			m.probeState.scroll = maxScroll(len(m.probeState.logs), m.getProbeVisibleRows())
			m.probeState.autoScroll = true
		}
	case "pgup":
		if m.probeState != nil && len(m.probeState.logs) > 0 {
			pageSize := m.getProbeVisibleRows()
			if m.probeState.selectedIdx > pageSize {
				m.probeState.selectedIdx -= pageSize
			} else {
				m.probeState.selectedIdx = 0
			}
			m.probeState.scroll = clampScrollToContain(m.probeState.scroll, m.probeState.selectedIdx, m.getProbeVisibleRows(), len(m.probeState.logs))
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
			m.probeState.scroll = clampScrollToContain(m.probeState.scroll, m.probeState.selectedIdx, m.getProbeVisibleRows(), logCount)
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
	// Header: title (2 lines), status (2 lines)
	// Footer: help (2 lines), error/success (2 lines)
	reserved := 8
	visible := m.height - reserved
	if visible < 5 {
		visible = 5
	}
	return visible
}

// handleServerLogsKeys handles keys in the server logs view
func (m *TUIModel) handleServerLogsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		m.viewMode = ViewList
		m.serverLogsScroll = 0
	case "g":
		// Jump to top
		m.serverLogsSelectedIdx = 0
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
	case "G":
		// Jump to bottom
		if len(m.logEntries) > 0 {
			m.serverLogsSelectedIdx = len(m.logEntries) - 1
			m.adjustServerLogsScroll()
		}
	case "R":
		// Reset dropped logs counter
		m.droppedLogs = 0
		m.successMsg = "Dropped logs reset"
		m.successTime = time.Now()
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
	total := len(m.logEntries)
	if total == 0 {
		m.serverLogsSelectedIdx = 0
		m.serverLogsScroll = 0
		return
	}
	m.serverLogsSelectedIdx = clamp(m.serverLogsSelectedIdx, 0, total-1)
	m.serverLogsScroll = clampScrollToContain(m.serverLogsScroll, m.serverLogsSelectedIdx, visibleRows, total)
}

// RunTUI starts the TUI with the provided socket path
func RunTUI(ctx context.Context, socketPath string) error {
	// We'll call the main package to create the API client
	// This will be implemented when we update the CLI
	return fmt.Errorf("TUI not yet integrated - use Run() with APIClient interface")
}

// handleSaveConfirmKeys handles keys in the save confirmation view
func (m *TUIModel) handleSaveConfirmKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.saveState == nil {
		m.viewMode = ViewProbe
		return m, nil
	}

	if m.saveState.editing {
		// Editing mode
		switch msg.Type {
		case tea.KeyEsc:
			// Stop editing
			m.saveState.editing = false
		case tea.KeyEnter:
			// Confirm with current path
			m.saveState.editing = false
		case tea.KeyBackspace:
			// Delete character before cursor
			if m.saveState.cursorPos > 0 {
				path := m.saveState.filePath
				m.saveState.filePath = path[:m.saveState.cursorPos-1] + path[m.saveState.cursorPos:]
				m.saveState.cursorPos--
			}
		case tea.KeyDelete:
			// Delete character at cursor
			if m.saveState.cursorPos < len(m.saveState.filePath) {
				path := m.saveState.filePath
				m.saveState.filePath = path[:m.saveState.cursorPos] + path[m.saveState.cursorPos+1:]
			}
		case tea.KeyLeft:
			if m.saveState.cursorPos > 0 {
				m.saveState.cursorPos--
			}
		case tea.KeyRight:
			if m.saveState.cursorPos < len(m.saveState.filePath) {
				m.saveState.cursorPos++
			}
		case tea.KeyHome:
			m.saveState.cursorPos = 0
		case tea.KeyEnd:
			m.saveState.cursorPos = len(m.saveState.filePath)
		case tea.KeyRunes:
			// Insert runes at cursor (basic rune-aware insertion)
			path := m.saveState.filePath
			for _, r := range msg.Runes {
				s := string(r)
				path = path[:m.saveState.cursorPos] + s + path[m.saveState.cursorPos:]
				m.saveState.cursorPos += len(s)
			}
			m.saveState.filePath = path
		}
	} else {
		// Not editing mode
		switch msg.String() {
		case "ctrl+c", "esc", "n", "q":
			// Cancel save
			m.viewMode = m.saveState.previousView
			m.saveState = nil
		case "e":
			// Start editing
			m.saveState.editing = true
			m.saveState.overwriteConfirm = false
		case "enter":
			// If file exists and not yet confirmed, ask for overwrite confirmation
			if m.saveState != nil {
				path := m.saveState.filePath
				if !m.saveState.overwriteConfirm {
					if _, err := os.Stat(path); err == nil {
						// File exists, require explicit confirmation
						m.saveState.overwriteConfirm = true
						return m, nil
					}
				}
			}
			// Proceed to save
			return m, m.saveProbeLogsToFile()
		}
	}

	return m, nil
}
