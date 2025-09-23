package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Styles for rendering
var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			MarginBottom(1)

	tableStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))

	selectedRowStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("62")).
				Foreground(lipgloss.Color("230")).
				Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Margin(1, 0)
)

// View renders the current TUI state
func (m *TUIModel) View() string {
	switch m.viewMode {
	case ViewList:
		return m.renderListView()
	case ViewDetail:
		return m.renderDetailView()
	case ViewConfirm:
		return m.renderConfirmView()
	case ViewProbe:
		return m.renderProbeView()
	default:
		return "Unknown view"
	}
}

// renderListView renders the main client list view
func (m *TUIModel) renderListView() string {
	var b strings.Builder
	header := m.renderHeader()
	table := m.renderTable()
	help := m.renderHelp()

	b.WriteString(header)
	b.WriteString("\n\n")
	b.WriteString(table)
	b.WriteString("\n")
	b.WriteString(help)

	// Show error messages
	if m.err != nil && time.Since(m.errorTime) < 5*time.Second {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
		errMsg := fmt.Sprintf("Error: %v", m.err)
		if len(errMsg) > 80 {
			lines := []string{}
			for i := 0; i < len(errMsg); i += 80 {
				end := i + 80
				if end > len(errMsg) {
					end = len(errMsg)
				}
				lines = append(lines, errMsg[i:end])
			}
			errMsg = strings.Join(lines, "\n")
		}
		b.WriteString("\n" + errorStyle.Render(errMsg))
	} else if m.err != nil && time.Since(m.errorTime) >= 5*time.Second {
		m.err = nil
	}

	// Show success messages
	if m.successMsg != "" && time.Since(m.successTime) < 3*time.Second {
		successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true)
		b.WriteString("\n" + successStyle.Render(m.successMsg))
	} else if m.successMsg != "" && time.Since(m.successTime) >= 3*time.Second {
		m.successMsg = ""
	}

	return b.String()
}

// renderHeader renders the header with server statistics
func (m *TUIModel) renderHeader() string {
	activeCount := 0
	for _, client := range m.clients {
		if client.Status == "active" {
			activeCount++
		}
	}

	headerText := fmt.Sprintf("Active Clients: %d  Total: %d  Last Update: %s",
		activeCount, len(m.clients), m.lastUpdate.Format("15:04:05"))

	return headerStyle.Render(headerText)
}

// renderTable renders the client table with scrolling support
func (m *TUIModel) renderTable() string {
	if len(m.clients) == 0 {
		return tableStyle.Render("No clients connected")
	}

	var rows []string

	header := "ID       User     VHost    Address         Status    Connected  Methods  Frames"
	rows = append(rows, header)
	rows = append(rows, strings.Repeat("─", len(header)))

	// Calculate visible range
	visibleRows := m.getVisibleRows()
	startIdx := m.listScroll
	endIdx := startIdx + visibleRows
	if endIdx > len(m.clients) {
		endIdx = len(m.clients)
	}

	// Show only visible clients
	for i := startIdx; i < endIdx; i++ {
		client := m.clients[i]
		row := m.formatClientRow(client, i == m.selectedIdx)
		if i == m.selectedIdx {
			row = selectedRowStyle.Render(row)
		}
		rows = append(rows, row)
	}

	content := strings.Join(rows, "\n")

	// Add scroll indicator if there are more clients
	var scrollInfo string
	if len(m.clients) > visibleRows {
		totalPages := (len(m.clients) + visibleRows - 1) / visibleRows
		currentPage := (m.listScroll / visibleRows) + 1
		scrollInfo = fmt.Sprintf(" [%d-%d of %d clients, page %d/%d]",
			startIdx+1, endIdx, len(m.clients), currentPage, totalPages)
	}

	result := tableStyle.Render(content)
	if scrollInfo != "" {
		scrollStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
		result += "\n" + scrollStyle.Render(scrollInfo)
	}

	return result
}

// renderDetailView renders the detailed client information view
func (m *TUIModel) renderDetailView() string {
	if m.clientDetail == nil {
		return "No client selected"
	}

	var b strings.Builder
	client := m.clientDetail

	// Header
	b.WriteString(headerStyle.Render(fmt.Sprintf("Client Details: %s", formatID(client.ID))))
	b.WriteString("\n\n")

	// Basic information
	b.WriteString(fmt.Sprintf("ID: %s\n", client.ID))
	b.WriteString(fmt.Sprintf("User: %s\n", client.User))
	b.WriteString(fmt.Sprintf("Virtual Host: %s\n", client.VirtualHost))
	b.WriteString(fmt.Sprintf("Address: %s\n", client.ClientAddress))
	b.WriteString(fmt.Sprintf("Status: %s\n", formatStatus(client.Status)))
	if client.ShutdownReason != "" {
		b.WriteString(fmt.Sprintf("Shutdown Reason: %s\n", client.ShutdownReason))
	}
	b.WriteString(fmt.Sprintf("Connected: %s (%s)\n",
		client.ConnectedAt.Format("2006-01-02 15:04:05"),
		formatDuration(time.Since(client.ConnectedAt))))
	b.WriteString(fmt.Sprintf("Client Banner: %s\n", client.ClientBanner))

	// Client Properties
	if len(client.ClientProperties) > 0 {
		b.WriteString("\nClient Properties:\n")

		// Sort properties by key for stable display
		var keys []string
		for key := range client.ClientProperties {
			if key != "" {
				keys = append(keys, key)
			}
		}
		sort.Stable(sort.StringSlice(keys))

		for _, key := range keys {
			value := client.ClientProperties[key]
			b.WriteString(fmt.Sprintf("  %s: %v\n", key, value))
		}
	}

	// Statistics
	if client.Stats != nil {
		stats := client.Stats
		b.WriteString("\nStatistics:\n")
		b.WriteString(fmt.Sprintf("  Started: %s (%s ago)\n",
			stats.StartedAt.Format("2006-01-02 15:04:05"), stats.Duration))
		b.WriteString(fmt.Sprintf("  Total Methods: %s\n", formatNumber(stats.TotalMethods)))
		b.WriteString(fmt.Sprintf("  Received Frames: %s\n", formatNumber(stats.ReceivedFrames)))
		b.WriteString(fmt.Sprintf("  Sent Frames: %s\n", formatNumber(stats.SentFrames)))
		b.WriteString(fmt.Sprintf("  Total Frames: %s\n", formatNumber(stats.TotalFrames)))

		// Method breakdown
		if len(stats.Methods) > 0 {
			b.WriteString("\nMethod Statistics:\n")

			// Sort methods by count (descending) then by name for stable display
			type methodCount struct {
				method string
				count  int64
			}
			var methods []methodCount
			for method, count := range stats.Methods {
				methods = append(methods, methodCount{method, count})
			}
			sort.SliceStable(methods, func(i, j int) bool {
				if methods[i].count != methods[j].count {
					return methods[i].count > methods[j].count
				}
				return methods[i].method < methods[j].method
			})

			// Show top 5 methods
			topCount := len(methods)
			if topCount > 5 {
				topCount = 5
			}
			for i := 0; i < topCount; i++ {
				mc := methods[i]
				b.WriteString(fmt.Sprintf("  %s: %d\n", mc.method, mc.count))
			}
		}
	}

	b.WriteString("\n")
	helpText := "Press ESC/q to go back • ↑↓/kj to scroll • Home/End • p probe • Shift+K shutdown"
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(helpText))

	// Implement scrolling
	content := b.String()
	lines := strings.Split(content, "\n")

	// Calculate available height (subtract some for header/footer margins)
	availableHeight := m.height - 4
	if availableHeight < 10 {
		availableHeight = 10
	}

	// Limit scroll position
	maxScroll := len(lines) - availableHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.detailScroll > maxScroll {
		m.detailScroll = maxScroll
	}

	// Apply scrolling
	startLine := m.detailScroll
	endLine := startLine + availableHeight
	if endLine > len(lines) {
		endLine = len(lines)
	}

	scrolledLines := lines[startLine:endLine]

	// Add scroll indicator if content is scrollable
	result := strings.Join(scrolledLines, "\n")
	if maxScroll > 0 {
		scrollInfo := fmt.Sprintf(" [%d/%d]", m.detailScroll+1, maxScroll+1)
		result += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(scrollInfo)
	}

	return result
}

// renderConfirmView renders the shutdown confirmation dialog
func (m *TUIModel) renderConfirmView() string {
	if m.confirmState == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("Confirm Shutdown\n\n")
	b.WriteString(m.confirmState.message)
	b.WriteString("\n\nPress 'y' to confirm, 'n' to cancel")
	return b.String()
}

// renderHelp renders the help text
func (m *TUIModel) renderHelp() string {
	help := "↑↓/kj navigate • PgUp/PgDn page • Enter info • p probe • Shift+K shutdown • r refresh • q quit"
	return helpStyle.Render(help)
}

// getVisibleRows calculates how many client rows can be displayed
func (m *TUIModel) getVisibleRows() int {
	// Calculate visible rows: total height - header - help - error/success area - margins
	headerLines := 3 // header + spacing
	helpLines := 2   // help + spacing
	marginLines := 4 // various margins and spacing
	visibleHeight := m.height - headerLines - helpLines - marginLines
	if visibleHeight < 5 {
		visibleHeight = 5
	}
	return visibleHeight
}

// renderProbeView renders the probe log streaming view
func (m *TUIModel) renderProbeView() string {
	if m.probeState == nil {
		return "No probe stream active"
	}

	var b strings.Builder

	// Header
	headerText := fmt.Sprintf("Probe Logs: %s", formatID(m.probeState.clientID))
	b.WriteString(headerStyle.Render(headerText))
	b.WriteString("\n\n")

	// Show log count
	logCount := len(m.probeState.logs)
	statusText := fmt.Sprintf("Total logs: %d", logCount)
	if logCount > 0 {
		latest := m.probeState.logs[logCount-1]
		statusText += fmt.Sprintf(" • Latest: %s", latest.Timestamp.Format("15:04:05.000"))
	}
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(statusText))
	b.WriteString("\n\n")

	// Render logs
	if logCount == 0 {
		b.WriteString("Waiting for probe logs...\n")
		// Debug: Show probe state details
		if m.probeState != nil {
			debugText := fmt.Sprintf("Debug: State initialized for %s", m.probeState.clientID)
			if m.probeState.logChan != nil {
				debugText += " • Channel ready"
			} else {
				debugText += " • Channel not ready"
			}
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render(debugText))
			b.WriteString("\n")
		}
	} else {
		visibleRows := m.getProbeVisibleRows()
		startIdx := m.probeState.scroll
		if startIdx > logCount-visibleRows {
			startIdx = logCount - visibleRows
		}
		if startIdx < 0 {
			startIdx = 0
		}
		endIdx := startIdx + visibleRows
		if endIdx > logCount {
			endIdx = logCount
		}

		// Render visible logs
		for i := startIdx; i < endIdx; i++ {
			log := m.probeState.logs[i]
			logLine := m.formatProbeLogLine(log)
			b.WriteString(logLine)
			b.WriteString("\n")
		}

		// Add scroll indicator
		if logCount > visibleRows {
			scrollInfo := fmt.Sprintf(" [%d-%d of %d logs]",
				startIdx+1, endIdx, logCount)
			scrollStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
			b.WriteString("\n" + scrollStyle.Render(scrollInfo))
		}
	}

	// Help text
	b.WriteString("\n")
	helpText := "Press ESC/q to go back • ↑↓/kj to scroll • Home/End • PgUp/PgDn page"
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(helpText))

	// Show error messages
	if m.err != nil && time.Since(m.errorTime) < 5*time.Second {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
		errMsg := fmt.Sprintf("Error: %v", m.err)
		b.WriteString("\n" + errorStyle.Render(errMsg))
	}

	return b.String()
}

// formatProbeLogLine formats a single probe log entry
func (m *TUIModel) formatProbeLogLine(log probeLogEntry) string {
	timestamp := log.Timestamp.Format("15:04:05.000")
	message := log.Message

	// Build attributes string
	var attrs []string
	if len(log.Attrs) > 0 {
		for k, v := range log.Attrs {
			attrs = append(attrs, fmt.Sprintf("%s=%v", k, v))
		}
	}

	// Combine parts
	result := fmt.Sprintf("%s %s", timestamp, message)
	if len(attrs) > 0 {
		attrStr := strings.Join(attrs, " ")
		// Truncate attributes if too long
		if len(attrStr) > 100 {
			attrStr = attrStr[:97] + "..."
		}
		result += " " + lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(attrStr)
	}

	return result
}

// adjustScrollForSelection adjusts scroll position to keep selected item visible
func (m *TUIModel) adjustScrollForSelection() {
	visibleRows := m.getVisibleRows()

	// If selected item is above visible area, scroll up
	if m.selectedIdx < m.listScroll {
		m.listScroll = m.selectedIdx
	}

	// If selected item is below visible area, scroll down
	if m.selectedIdx >= m.listScroll+visibleRows {
		m.listScroll = m.selectedIdx - visibleRows + 1
	}

	// Ensure scroll is within bounds
	if m.listScroll < 0 {
		m.listScroll = 0
	}
	maxScroll := len(m.clients) - visibleRows
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.listScroll > maxScroll {
		m.listScroll = maxScroll
	}
}
