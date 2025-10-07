package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	runewidth "github.com/mattn/go-runewidth"
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

	selectedStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("238")). // Darker gray for probe log selection
			Foreground(lipgloss.Color("255"))  // Bright white text

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Margin(1, 0)

	// Cached frequently used styles
	errorStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	successStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true)
	scrollInfoStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	statusDimStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	debugDimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	attrGreyStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	cursorStyle     = lipgloss.NewStyle().Background(lipgloss.Color("255")).Foreground(lipgloss.Color("0")).Render(" ")
	logPaneBoxStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)

	// Log level styles
	levelErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	levelWarnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	levelInfoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	levelDefaultStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
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
	case ViewServerLogs:
		return m.renderServerLogsView()
	case ViewSaveConfirm:
		return m.renderSaveConfirmView()
	default:
		return "Unknown view"
	}
}

// renderListView renders the main client list view
func (m *TUIModel) renderListView() string {
	var b strings.Builder
	header := m.renderHeader()
	table := m.renderTable()
	logs := m.renderLogPane()
	help := m.renderHelp()

	b.WriteString(header)
	b.WriteString("\n\n")
	b.WriteString(table)
	b.WriteString("\n")
	b.WriteString(logs)
	b.WriteString("\n")
	b.WriteString(help)

	// Show error messages
	if m.err != nil && time.Since(m.errorTime) < errorToastDuration {
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
	if m.successMsg != "" && time.Since(m.successTime) < successToastDuration {
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
	// Show dropped server logs if any
	if m.droppedLogs > 0 {
		headerText += fmt.Sprintf("  Dropped logs: %d", m.droppedLogs)
	}

	return headerStyle.Render(headerText)
}

// renderTable renders the client table with scrolling support
func (m *TUIModel) renderTable() string {
	if len(m.clients) == 0 {
		return tableStyle.Render("No clients connected")
	}

	var rows []string

	header := "ID         User     VHost    Address              Status    Connected  Methods  Frames"
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
		result += "\n" + scrollInfoStyle.Render(scrollInfo)
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
	b.WriteString(helpStyle.Render(helpText))

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
	help := "↑↓/kj navigate • PgUp/PgDn page • Enter info • p probe • l logs • Shift+K shutdown • r refresh • q quit"
	return helpStyle.Render(help)
}

// getVisibleRows calculates how many client rows can be displayed
func (m *TUIModel) getVisibleRows() int {
	// Calculate visible rows: total height - header - help - log pane - error/success area - margins
	headerLines := 3 // header + spacing
	helpLines := 2   // help + spacing

	// Calculate log pane lines dynamically
	logPaneLines := m.estimateLogPaneLines()

	marginLines := 4 // various margins and spacing
	visibleHeight := m.height - headerLines - helpLines - logPaneLines - marginLines
	if visibleHeight < 5 {
		visibleHeight = 5
	}
	return visibleHeight
}

// estimateLogPaneLines estimates the number of lines the log pane will occupy
func (m *TUIModel) estimateLogPaneLines() int {
	if len(m.logEntries) == 0 {
		return 3 // Border + "Logs (waiting...)" + spacing
	}

	// Count lines for last 3 log entries
	displayCount := 3
	startIdx := len(m.logEntries) - displayCount
	if startIdx < 0 {
		startIdx = 0
	}

	lineCount := 1       // Title line: "Logs (N total):"
	width := m.width - 4 // Account for border padding
	if width <= 0 {
		width = 80
	}

	for i := startIdx; i < len(m.logEntries); i++ {
		entry := m.logEntries[i]
		// Estimate lines for this entry
		logText := m.formatLogEntry(entry, false)
		// Count newlines in formatted log
		lineCount += strings.Count(logText, "\n") + 1
	}

	return lineCount + 4 // + border lines and spacing
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
	// Debug: Show scroll position
	visibleRows := m.getProbeVisibleRows()
	maxScroll := logCount - visibleRows
	if maxScroll < 0 {
		maxScroll = 0
	}
	statusText += fmt.Sprintf(" • Scroll: %d/%d (visible: %d)", m.probeState.scroll, maxScroll, visibleRows)
	b.WriteString(statusDimStyle.Render(statusText))
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
			b.WriteString(debugDimStyle.Render(debugText))
			b.WriteString("\n")
		}
	} else {
		maxDisplayLines := m.getProbeVisibleRows()

		// Use common rendering function
		result := renderLogsWithWrapping(
			logCount,
			maxDisplayLines,
			m.probeState.scroll,
			m.probeState.selectedIdx,
			func(idx int, isSelected bool) string {
				log := m.probeState.logs[idx]
				logLine := m.formatProbeLogLine(log)
				if isSelected {
					logLine = selectedStyle.Render(logLine)
				}
				return logLine
			},
		)

		// Write rendered logs
		for _, logLine := range result.renderedLines {
			b.WriteString(logLine)
			b.WriteString("\n")
		}

		// Add scroll indicator
		if logCount > len(result.renderedLines) {
			scrollInfo := fmt.Sprintf(" [%d-%d of %d logs]",
				result.startIdx+1, result.endIdx, logCount)
			b.WriteString("\n" + scrollInfoStyle.Render(scrollInfo))
		}
	}

	// Help text
	b.WriteString("\n")
	helpText := "Press ESC/q to go back • ↑↓/kj to scroll • Home/End • PgUp/PgDn page • SPACE pause • s save"
	if m.probeState != nil {
		if m.probeState.autoScroll {
			helpText += " • Auto-scroll: ON"
		} else {
			helpText += " • Auto-scroll: OFF"
		}
	}
	b.WriteString(helpStyle.Render(helpText))

	// Show error messages
	if m.err != nil && time.Since(m.errorTime) < errorToastDuration {
		errMsg := fmt.Sprintf("Error: %v", m.err)
		b.WriteString("\n" + errorStyle.Render(errMsg))
	}

	// Show success messages (short toast)
	if m.successMsg != "" && time.Since(m.successTime) < successToastDuration {
		b.WriteString("\n" + successStyle.Render(m.successMsg))
	}

	return b.String()
}

// formatProbeLogLine formats a single probe log entry
func (m *TUIModel) formatProbeLogLine(log probeLogEntry) string {
	timestamp := log.Timestamp.Format("15:04:05.000")
	message := log.Message

	// Build the main text first (without styling)
	mainText := fmt.Sprintf("%s %s", timestamp, message)

	var attrStr string
	if len(log.Attrs) > 0 {
		// Marshal attrs to JSON for consistent display with CLI
		attrs, err := json.Marshal(log.Attrs)
		if err == nil {
			attrStr = string(attrs)
		}
	}

	// Calculate available width for wrapping
	width := m.width - 2 // -2 for margins
	if width <= 0 {
		width = 80 // default width
	}

	// Combine and wrap the full text
	fullText := mainText
	if attrStr != "" {
		fullText += " " + attrStr
	}

	// Wrap to screen width
	lines := wrapText(fullText, width)

	// Apply styling to the attrs portion of the first line and subsequent wrapped lines
	// For simplicity, apply grey color to everything after the message on each line
	if attrStr != "" {
		styledLines := make([]string, len(lines))
		attrStartInFirstLine := len(mainText) + 1 // +1 for space before attrs

		for i, line := range lines {
			if i == 0 {
				// First line: style only the attrs part if it fits
				if len(line) > attrStartInFirstLine {
					styledLines[i] = line[:attrStartInFirstLine] +
						attrGreyStyle.Render(line[attrStartInFirstLine:])
				} else {
					styledLines[i] = line
				}
			} else {
				// Continuation lines: style the entire line (it's part of attrs)
				styledLines[i] = attrGreyStyle.Render(line)
			}
		}
		return strings.Join(styledLines, "\n")
	}

	return strings.Join(lines, "\n")
}

// renderResult holds the result of rendering logs with line wrapping
type renderResult struct {
	renderedLines []string // formatted and styled log lines ready to display
	startIdx      int      // first log entry index in the result
	endIdx        int      // last log entry index + 1 in the result
}

// renderLogsWithWrapping renders log entries accounting for line wrapping,
// ensuring the selected entry is always visible within maxDisplayLines.
// formatFunc should return the formatted (and optionally styled) log line for a given index.
func renderLogsWithWrapping(
	logCount int,
	maxDisplayLines int,
	scrollPos int,
	selectedIdx int,
	formatFunc func(idx int, isSelected bool) string,
) renderResult {
	if logCount == 0 {
		return renderResult{}
	}

	// Clamp inputs
	startIdx := scrollPos
	if startIdx < 0 {
		startIdx = 0
	}
	if startIdx >= logCount {
		startIdx = logCount - 1
	}
	if startIdx < 0 {
		startIdx = 0
	}

	if selectedIdx < 0 {
		selectedIdx = 0
	}
	if selectedIdx >= logCount {
		selectedIdx = logCount - 1
	}

	// If selected is before startIdx, adjust startIdx to show selected
	if selectedIdx < startIdx {
		startIdx = selectedIdx
	}

	// Render logs from startIdx, counting actual display lines
	var renderedLines []string
	var renderedIndices []int
	displayLines := 0
	endIdx := startIdx

	for i := startIdx; i < logCount && displayLines < maxDisplayLines; i++ {
		logLine := formatFunc(i, false)
		lineCount := strings.Count(logLine, "\n") + 1

		// If adding this entry would exceed available space, stop unless it's the selected entry
		if displayLines+lineCount > maxDisplayLines && i != selectedIdx {
			// If we haven't reached the selected entry yet, we need to adjust
			if selectedIdx > i {
				// Skip this entry and continue to reach selected
				continue
			}
			break
		}

		// Re-format with selection styling if this is the selected entry
		if i == selectedIdx {
			logLine = formatFunc(i, true)
		}

		renderedLines = append(renderedLines, logLine)
		renderedIndices = append(renderedIndices, i)
		displayLines += lineCount
		endIdx = i + 1
	}

	// If we didn't render the selected entry, adjust startIdx and retry
	selectedRendered := false
	for _, idx := range renderedIndices {
		if idx == selectedIdx {
			selectedRendered = true
			break
		}
	}

	if !selectedRendered && selectedIdx < logCount {
		// Start from selected entry and render backwards/forwards to fill screen
		renderedLines = []string{}
		renderedIndices = []int{}
		displayLines = 0

		// First, render the selected entry
		logLine := formatFunc(selectedIdx, true)
		lineCount := strings.Count(logLine, "\n") + 1

		renderedLines = append(renderedLines, logLine)
		renderedIndices = append(renderedIndices, selectedIdx)
		displayLines += lineCount

		// Try to add entries after selected
		for i := selectedIdx + 1; i < logCount && displayLines < maxDisplayLines; i++ {
			logLine := formatFunc(i, false)
			lineCount := strings.Count(logLine, "\n") + 1

			if displayLines+lineCount > maxDisplayLines {
				break
			}

			renderedLines = append(renderedLines, logLine)
			renderedIndices = append(renderedIndices, i)
			displayLines += lineCount
		}

		// Try to add entries before selected
		var beforeLines []string
		var beforeIndices []int
		for i := selectedIdx - 1; i >= 0 && displayLines < maxDisplayLines; i-- {
			logLine := formatFunc(i, false)
			lineCount := strings.Count(logLine, "\n") + 1

			if displayLines+lineCount > maxDisplayLines {
				break
			}

			beforeLines = append([]string{logLine}, beforeLines...)
			beforeIndices = append([]int{i}, beforeIndices...)
			displayLines += lineCount
		}

		renderedLines = append(beforeLines, renderedLines...)
		renderedIndices = append(beforeIndices, renderedIndices...)

		if len(renderedIndices) > 0 {
			startIdx = renderedIndices[0]
			endIdx = renderedIndices[len(renderedIndices)-1] + 1
		}
	}

	return renderResult{
		renderedLines: renderedLines,
		startIdx:      startIdx,
		endIdx:        endIdx,
	}
}

// wrapText wraps text to the specified width, returning a slice of lines
// Text is wrapped at the right edge regardless of character type (no word wrapping)
func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}

	// Split by existing newlines and wrap each segment
	segments := strings.Split(text, "\n")
	var out []string
	for _, seg := range segments {
		if seg == "" {
			out = append(out, "")
			continue
		}

		runes := []rune(seg)
		start := 0
		lineWidth := 0

		for i := 0; i < len(runes); i++ {
			ch := runes[i]
			rw := runewidth.RuneWidth(ch)

			// If adding this rune would exceed width, wrap here
			if lineWidth+rw > width {
				out = append(out, string(runes[start:i]))
				start = i
				lineWidth = 0
			}

			lineWidth += rw
		}

		// Flush remainder
		if start < len(runes) {
			out = append(out, string(runes[start:]))
		}
	}
	return out
}

// renderLogPane renders the log pane showing recent logs
func (m *TUIModel) renderLogPane() string {
	var b strings.Builder

	var content string
	if len(m.logEntries) == 0 {
		content = debugDimStyle.Render("Logs (waiting...)")
	} else {
		// Show last 3 log entries
		displayCount := 3
		startIdx := len(m.logEntries) - displayCount
		if startIdx < 0 {
			startIdx = 0
		}

		var logLines []string
		logLines = append(logLines, debugDimStyle.Render(fmt.Sprintf("Logs (%d total):", len(m.logEntries))))
		for i := startIdx; i < len(m.logEntries); i++ {
			entry := m.logEntries[i]
			logLine := m.formatLogEntry(entry, false)
			logLines = append(logLines, logLine)
		}
		content = strings.Join(logLines, "\n")
	}

	b.WriteString(logPaneBoxStyle.Render(content))

	return b.String()
}

// formatLogEntry formats a log entry for display
func (m *TUIModel) formatLogEntry(entry LogEntry, isSelected bool) string {
	timestamp := entry.Time.Format("15:04:05")

	// Build the main text first (without styling)
	mainText := fmt.Sprintf("%s %-5s %s", timestamp, entry.Level, entry.Message)

	attrStr := entry.AttrJSON

	// Calculate available width for wrapping
	width := m.width - 2 // -2 for margins
	if width <= 0 {
		width = 80 // default width
	}

	// Combine and wrap the full text
	fullText := mainText
	if attrStr != "" {
		fullText += " " + attrStr
	}

	// Wrap to screen width
	lines := wrapText(fullText, width)

	// Apply styling to the level and attrs portions
	styledLines := make([]string, len(lines))

	// Color by level
	var levelStyle lipgloss.Style
	switch entry.Level {
	case "ERROR":
		levelStyle = levelErrorStyle
	case "WARN":
		levelStyle = levelWarnStyle
	case "INFO":
		levelStyle = levelInfoStyle
	default:
		levelStyle = levelDefaultStyle
	}

	attrStyle := attrGreyStyle

	// If selected, apply selection background to all styles
	if isSelected {
		levelStyle = levelStyle.Inherit(selectedStyle)
		attrStyle = attrStyle.Inherit(selectedStyle)
	}

	for i, line := range lines {
		if i == 0 {
			// First line: apply level color to the level part
			// Format: "HH:MM:SS LEVEL message {attrs...}"
			// Level is at position 9-13 (after timestamp and space)
			if len(line) > 14 {
				timestampPart := line[:9]
				if isSelected {
					timestampPart = selectedStyle.Render(timestampPart)
				}
				levelPart := levelStyle.Render(line[9:14])
				rest := line[14:]
				// Find where attrs start (after message)
				attrStartInLine := len(mainText) - 14 // position relative to after level
				var restStyled string
				if attrStr != "" && len(rest) > attrStartInLine {
					messagePart := rest[:attrStartInLine]
					attrPart := rest[attrStartInLine:]
					if isSelected {
						messagePart = selectedStyle.Render(messagePart)
					}
					restStyled = messagePart + attrStyle.Render(attrPart)
				} else {
					if isSelected {
						restStyled = selectedStyle.Render(rest)
					} else {
						restStyled = rest
					}
				}
				styledLines[i] = timestampPart + levelPart + restStyled
			} else {
				if isSelected {
					styledLines[i] = selectedStyle.Render(line)
				} else {
					styledLines[i] = line
				}
			}
		} else {
			// Continuation lines: style as attrs
			styledLines[i] = attrStyle.Render(line)
		}
	}

	return strings.Join(styledLines, "\n")
}

// adjustScrollForSelection adjusts scroll position to keep selected item visible
func (m *TUIModel) adjustScrollForSelection() {
	visibleRows := m.getVisibleRows()
	total := len(m.clients)
	// Clamp selected index to available range
	if total == 0 {
		m.selectedIdx = 0
		m.listScroll = 0
		return
	}
	m.selectedIdx = clamp(m.selectedIdx, 0, total-1)
	// Adjust scroll to contain selection and clamp bounds
	m.listScroll = clampScrollToContain(m.listScroll, m.selectedIdx, visibleRows, total)
}

// renderServerLogsView renders the server logs view
func (m *TUIModel) renderServerLogsView() string {
	var b strings.Builder

	// Header
	headerText := fmt.Sprintf("Server Logs (%d total)", len(m.logEntries))
	if m.droppedLogs > 0 {
		headerText += fmt.Sprintf(" • dropped %d", m.droppedLogs)
	}
	b.WriteString(headerStyle.Render(headerText))
	b.WriteString("\n\n")

	// Show logs
	if len(m.logEntries) == 0 {
		b.WriteString("No server logs yet...\n")
	} else {
		logCount := len(m.logEntries)
		maxDisplayLines := m.getServerLogsVisibleRows()

		// Use common rendering function
		result := renderLogsWithWrapping(
			logCount,
			maxDisplayLines,
			m.serverLogsScroll,
			m.serverLogsSelectedIdx,
			func(idx int, isSelected bool) string {
				entry := m.logEntries[idx]
				return m.formatLogEntry(entry, isSelected)
			},
		)

		// Write rendered logs
		for _, logLine := range result.renderedLines {
			b.WriteString(logLine)
			b.WriteString("\n")
		}

		// Add scroll indicator
		if logCount > len(result.renderedLines) {
			scrollInfo := fmt.Sprintf(" [%d-%d of %d logs]",
				result.startIdx+1, result.endIdx, logCount)
			scrollStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
			b.WriteString("\n" + scrollStyle.Render(scrollInfo))
		}
	}

	// Help text
	b.WriteString("\n")
	helpText := "Press ESC/q to go back • ↑↓/kj to scroll • Home/End • PgUp/PgDn page"
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(helpText))

	return b.String()
}

// renderSaveConfirmView renders the save file confirmation view
func (m *TUIModel) renderSaveConfirmView() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("Save Probe Logs to File"))
	b.WriteString("\n\n")

	if m.saveState == nil {
		return "Error: save state not initialized"
	}

	// Show file path with editing capability
	b.WriteString("File path: ")
	if m.saveState.editing {
		// Show cursor when editing
		before := m.saveState.filePath[:m.saveState.cursorPos]
		after := m.saveState.filePath[m.saveState.cursorPos:]
		cursor := lipgloss.NewStyle().Background(lipgloss.Color("255")).Foreground(lipgloss.Color("0")).Render(" ")
		b.WriteString(before + cursor + after)
	} else {
		b.WriteString(m.saveState.filePath)
	}
	b.WriteString("\n\n")

	// Show log count
	if m.probeState != nil {
		b.WriteString(fmt.Sprintf("Logs to save: %d entries\n\n", len(m.probeState.logs)))
	}

	// Help text
	if m.saveState.editing {
		helpText := "Type to edit path • ESC to stop editing • ENTER to confirm"
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(helpText))
	} else {
		helpText := "ENTER to save • e to edit path • ESC/n to cancel"
		// If an overwrite confirmation is required, adjust messaging
		if m.saveState.overwriteConfirm {
			helpText = "File exists: ENTER to overwrite • e edit path • ESC/n cancel"
		}
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(helpText))
	}

	return b.String()
}
