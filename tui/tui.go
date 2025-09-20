package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the TUI application
func Run(ctx context.Context, apiClient APIClient) error {
	model := NewModel(ctx, apiClient)
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

// RunTUI starts the TUI with the provided socket path
func RunTUI(ctx context.Context, socketPath string) error {
	// We'll call the main package to create the API client
	// This will be implemented when we update the CLI
	return fmt.Errorf("TUI not yet integrated - use Run() with APIClient interface")
}