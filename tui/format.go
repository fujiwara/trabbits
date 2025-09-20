package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/fujiwara/trabbits/types"
)

// Styles for formatting
var (
	activeStatusStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("46"))

	shutdownStatusStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196"))
)

// formatID formats a client ID for display
func formatID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:6] + ".."
}

// formatAddress formats a client address for display
func formatAddress(addr string) string {
	if len(addr) <= 15 {
		return addr
	}
	return addr[:15]
}

// formatStatus formats a client status with color
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

// formatDuration formats a duration in a human-readable way
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

// formatNumber formats a number with K/M/G suffixes
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

// truncate truncates a string to maxLen with ".." suffix
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-2] + ".."
}

// getStatValue safely extracts a stat value from StatsSummary
func getStatValue(stats *types.StatsSummary, field string) int64 {
	if stats == nil {
		return 0
	}
	switch field {
	case "total_methods":
		return stats.TotalMethods
	case "received_frames":
		return stats.ReceivedFrames
	case "sent_frames":
		return stats.SentFrames
	case "total_frames":
		return stats.TotalFrames
	default:
		return 0
	}
}

// formatClientRow formats a client row for the table display
func (m *TUIModel) formatClientRow(client types.ClientInfo, selected bool) string {
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