package trabbits

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tuiModel struct {
	ctx          context.Context
	apiClient    *apiClient
	clients      []ClientInfo
	selectedIdx  int
	viewMode     viewMode
	selectedID   string
	clientDetail *FullClientInfo
	confirmState *confirmState
	width        int
	height       int
	lastUpdate   time.Time
	err          error
	errorTime    time.Time
	successMsg   string
	successTime  time.Time
	detailScroll int
}

type viewMode int

const (
	viewList viewMode = iota
	viewDetail
	viewConfirm
)

type confirmState struct {
	clientID string
	message  string
}

type tickMsg time.Time
type clientsMsg []ClientInfo
type clientDetailMsg *FullClientInfo
type errorMsg error
type successMsg string

var (
	headerStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)

	tableStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))

	selectedRowStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("62")).
				Foreground(lipgloss.Color("230")).
				Bold(true)

	activeStatusStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("46"))

	shutdownStatusStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Margin(1, 0)
)

func newTUIModel(ctx context.Context, apiClient *apiClient) *tuiModel {
	return &tuiModel{
		ctx:       ctx,
		apiClient: apiClient,
		clients:   []ClientInfo{},
		viewMode:  viewList,
	}
}

func (m *tuiModel) Init() tea.Cmd {
	return tea.Batch(
		m.fetchClients(),
		tick(),
	)
}

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			if m.viewMode == viewDetail && m.clientDetail != nil {
				cmds = append(cmds, m.fetchClientDetail(m.clientDetail.ID))
			}

			return m, tea.Batch(cmds...)
		}
		return m, tick()

	case clientsMsg:
		m.clients = []ClientInfo(msg)
		m.lastUpdate = time.Now()
		if m.selectedIdx >= len(m.clients) && len(m.clients) > 0 {
			m.selectedIdx = len(m.clients) - 1
		}
		return m, nil

	case clientDetailMsg:
		m.clientDetail = (*FullClientInfo)(msg)
		m.viewMode = viewDetail
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

func (m *tuiModel) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.viewMode {
	case viewList:
		return m.handleListKeys(msg)
	case viewDetail:
		return m.handleDetailKeys(msg)
	case viewConfirm:
		return m.handleConfirmKeys(msg)
	}
	return m, nil
}

func (m *tuiModel) handleListKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "K":
		if len(m.clients) > 0 && m.selectedIdx < len(m.clients) {
			client := m.clients[m.selectedIdx]
			// Debug: ensure we have the correct client ID
			if client.ID == "" {
				m.err = fmt.Errorf("selected client has empty ID (index: %d, total: %d)", m.selectedIdx, len(m.clients))
				m.errorTime = time.Now()
				return m, nil
			}
			m.confirmState = &confirmState{
				clientID: client.ID,
				message:  fmt.Sprintf("Shutdown client %s (%s@%s)?", formatID(client.ID), client.User, client.ClientAddress),
			}
			m.viewMode = viewConfirm
		}
	case "up", "k":
		if m.selectedIdx > 0 {
			m.selectedIdx--
		}
	case "down", "j":
		if m.selectedIdx < len(m.clients)-1 {
			m.selectedIdx++
		}
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

func (m *tuiModel) handleDetailKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		m.viewMode = viewList
		m.clientDetail = nil
		m.detailScroll = 0
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

func (m *tuiModel) handleConfirmKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q", "esc", "n":
		m.viewMode = viewList
		m.confirmState = nil
	case "y", "enter":
		if m.confirmState != nil {
			clientID := m.confirmState.clientID
			m.viewMode = viewList
			m.confirmState = nil
			return m, m.shutdownClient(clientID)
		}
	}
	return m, nil
}

func (m *tuiModel) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	switch m.viewMode {
	case viewList:
		return m.renderListView()
	case viewDetail:
		return m.renderDetailView()
	case viewConfirm:
		return m.renderConfirmView()
	}
	return ""
}

func (m *tuiModel) renderListView() string {
	var b strings.Builder

	header := m.renderHeader()
	table := m.renderTable()
	help := m.renderHelp()

	b.WriteString(header)
	b.WriteString("\n\n")
	b.WriteString(table)
	b.WriteString("\n")
	b.WriteString(help)

	if m.err != nil && time.Since(m.errorTime) < 5*time.Second {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
		// Split long error messages into multiple lines
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

	if m.successMsg != "" && time.Since(m.successTime) < 3*time.Second {
		successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true)
		b.WriteString("\n" + successStyle.Render(m.successMsg))
	} else if m.successMsg != "" && time.Since(m.successTime) >= 3*time.Second {
		m.successMsg = ""
	}

	return b.String()
}

func (m *tuiModel) renderHeader() string {
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

func (m *tuiModel) renderTable() string {
	if len(m.clients) == 0 {
		return tableStyle.Render("No clients connected")
	}

	var rows []string

	header := "ID       User     VHost    Address         Status    Connected  Methods  Frames"
	rows = append(rows, header)
	rows = append(rows, strings.Repeat("─", len(header)))

	for i, client := range m.clients {
		row := m.formatClientRow(client, i == m.selectedIdx)
		if i == m.selectedIdx {
			row = selectedRowStyle.Render(row)
		}
		rows = append(rows, row)
	}

	content := strings.Join(rows, "\n")
	return tableStyle.Render(content)
}

func (m *tuiModel) formatClientRow(client ClientInfo, selected bool) string {
	prefix := " "
	if selected {
		prefix = ">"
	}

	id := formatID(client.ID)
	user := truncate(client.User, 8)
	vhost := truncate(client.VirtualHost, 8)
	address := formatAddress(client.ClientAddress)
	status := formatStatus(client.Status)
	connected := formatDuration(time.Since(client.ConnectedAt))
	methods := formatNumber(getStatValue(client.Stats, "total_methods"))
	frames := formatNumber(getStatValue(client.Stats, "total_frames"))

	return fmt.Sprintf("%s %-8s %-8s %-8s %-15s %-8s %-10s %-8s %-8s",
		prefix, id, user, vhost, address, status, connected, methods, frames)
}

func (m *tuiModel) renderDetailView() string {
	if m.clientDetail == nil {
		return "Loading client details..."
	}

	var b strings.Builder

	// Header
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		MarginBottom(1)

	sectionStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("33")).
		MarginTop(1)

	b.WriteString(headerStyle.Render("Client Details"))
	b.WriteString("\n\n")

	// Basic Information
	b.WriteString(sectionStyle.Render("Basic Information:"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  ID: %s\n", m.clientDetail.ID))
	b.WriteString(fmt.Sprintf("  User: %s\n", m.clientDetail.User))
	b.WriteString(fmt.Sprintf("  Virtual Host: %s\n", m.clientDetail.VirtualHost))
	b.WriteString(fmt.Sprintf("  Address: %s\n", m.clientDetail.ClientAddress))
	b.WriteString(fmt.Sprintf("  Status: %s\n", formatStatus(m.clientDetail.Status)))
	b.WriteString(fmt.Sprintf("  Connected: %s\n", m.clientDetail.ConnectedAt.Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("  Duration: %s\n", time.Since(m.clientDetail.ConnectedAt).Round(time.Second)))
	b.WriteString(fmt.Sprintf("  Banner: %s\n", m.clientDetail.ClientBanner))

	if m.clientDetail.ShutdownReason != "" {
		b.WriteString(fmt.Sprintf("  Shutdown Reason: %s\n", m.clientDetail.ShutdownReason))
	}

	// Client Properties
	if len(m.clientDetail.ClientProperties) > 0 {
		b.WriteString("\n")
		b.WriteString(sectionStyle.Render("Client Properties:"))
		b.WriteString("\n")

		// Sort properties by key
		var propKeys []string
		for key := range m.clientDetail.ClientProperties {
			propKeys = append(propKeys, key)
		}
		sort.Stable(sort.StringSlice(propKeys))

		for _, key := range propKeys {
			b.WriteString(fmt.Sprintf("  %s: %v\n", key, m.clientDetail.ClientProperties[key]))
		}
	}

	// Statistics
	if m.clientDetail.Stats != nil {
		b.WriteString("\n")
		b.WriteString(sectionStyle.Render("Statistics:"))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  Started: %s\n", m.clientDetail.Stats.StartedAt.Format("2006-01-02 15:04:05")))
		b.WriteString(fmt.Sprintf("  Total Methods: %d\n", m.clientDetail.Stats.TotalMethods))
		b.WriteString(fmt.Sprintf("  Received Frames: %d\n", m.clientDetail.Stats.ReceivedFrames))
		b.WriteString(fmt.Sprintf("  Sent Frames: %d\n", m.clientDetail.Stats.SentFrames))
		b.WriteString(fmt.Sprintf("  Total Frames: %d\n", m.clientDetail.Stats.TotalFrames))
		b.WriteString(fmt.Sprintf("  Duration: %s\n", m.clientDetail.Stats.Duration))

		// Method breakdown
		if len(m.clientDetail.Stats.Methods) > 0 {
			b.WriteString("\n")
			b.WriteString(sectionStyle.Render("Method Breakdown (by name):"))
			b.WriteString("\n")

			// Sort methods by name for consistent display
			var methodNames []string
			for method := range m.clientDetail.Stats.Methods {
				methodNames = append(methodNames, method)
			}
			sort.Stable(sort.StringSlice(methodNames))

			for _, method := range methodNames {
				count := m.clientDetail.Stats.Methods[method]
				b.WriteString(fmt.Sprintf("  %s: %d\n", method, count))
			}

			// Also show top methods by usage
			if len(methodNames) > 3 {
				b.WriteString("\n")
				b.WriteString(sectionStyle.Render("Top Methods (by usage):"))
				b.WriteString("\n")

				// Sort methods by count for top usage
				type methodCount struct {
					method string
					count  int64
				}
				var methods []methodCount
				for method, count := range m.clientDetail.Stats.Methods {
					methods = append(methods, methodCount{method, count})
				}

				// Stable sort by count (descending), with method name as secondary sort for deterministic ordering
				sort.SliceStable(methods, func(i, j int) bool {
					if methods[i].count == methods[j].count {
						return methods[i].method < methods[j].method
					}
					return methods[i].count > methods[j].count
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
	}

	b.WriteString("\n")
	helpText := "Press ESC/q to go back • ↑↓/kj to scroll • Home/End"
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

func (m *tuiModel) renderConfirmView() string {
	if m.confirmState == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("Confirm Shutdown\n\n")
	b.WriteString(m.confirmState.message)
	b.WriteString("\n\nPress 'y' to confirm, 'n' to cancel")
	return b.String()
}

func (m *tuiModel) renderHelp() string {
	help := "↑↓/kj navigate • Enter info • Shift+K shutdown • r refresh • q quit"
	return helpStyle.Render(help)
}

func (m *tuiModel) fetchClients() tea.Cmd {
	return func() tea.Msg {
		clients, err := m.apiClient.getClients(m.ctx)
		if err != nil {
			return errorMsg(err)
		}
		return clientsMsg(clients)
	}
}

func (m *tuiModel) fetchClientDetail(clientID string) tea.Cmd {
	return func() tea.Msg {
		baseURL, err := url.Parse(m.apiClient.endpoint)
		if err != nil {
			return errorMsg(fmt.Errorf("invalid base URL: %w", err))
		}

		u, err := url.Parse(fmt.Sprintf("clients/%s", clientID))
		if err != nil {
			return errorMsg(fmt.Errorf("invalid client path: %w", err))
		}

		fullURL := baseURL.ResolveReference(u)

		req, _ := http.NewRequestWithContext(m.ctx, http.MethodGet, fullURL.String(), nil)
		resp, err := m.apiClient.client.Do(req)
		if err != nil {
			return errorMsg(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return errorMsg(fmt.Errorf("failed to get client details: %s", resp.Status))
		}

		var detail FullClientInfo
		if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
			return errorMsg(err)
		}
		return clientDetailMsg(&detail)
	}
}

func (m *tuiModel) shutdownClient(clientID string) tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			// Debug: log the actual client ID being used
			if err := m.apiClient.shutdownProxy(m.ctx, clientID, "TUI shutdown"); err != nil {
				return errorMsg(fmt.Errorf("shutdown failed for client %s (full ID: %s): %w", formatID(clientID), clientID, err))
			}
			return successMsg(fmt.Sprintf("Client %s shutdown successfully", formatID(clientID)))
		},
		func() tea.Msg {
			// Wait a moment then refresh the client list
			time.Sleep(500 * time.Millisecond)
			clients, err := m.apiClient.getClients(m.ctx)
			if err != nil {
				return errorMsg(fmt.Errorf("failed to refresh client list: %w", err))
			}
			return clientsMsg(clients)
		},
	)
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func formatID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:6] + ".."
}

func formatAddress(addr string) string {
	if len(addr) <= 15 {
		return addr
	}
	return addr[:15]
}

func formatStatus(status string) string {
	switch status {
	case "active":
		return activeStatusStyle.Render("Active")
	case "shutting_down":
		return shutdownStatusStyle.Render("Shutdown")
	default:
		return status
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	} else {
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func formatNumber(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	} else if n < 1000000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	} else if n < 1000000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	} else {
		return fmt.Sprintf("%.1fG", float64(n)/1000000000)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-2] + ".."
}

func getStatValue(stats *StatsSummary, field string) int64 {
	if stats == nil {
		return 0
	}
	switch field {
	case "total_methods":
		return stats.TotalMethods
	case "total_frames":
		return stats.TotalFrames
	default:
		return 0
	}
}

func runTUI(ctx context.Context, opt *CLI) error {
	client := newAPIClient(opt.APISocket)
	model := newTUIModel(ctx, client)

	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
