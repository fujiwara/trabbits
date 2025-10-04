package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/fujiwara/trabbits/types"
	"testing"
)

func TestClientsSelectionPreservedByID(t *testing.T) {
	m := &TUIModel{}
	// initial clients
	m.clients = []types.ClientInfo{{ID: "a1"}, {ID: "b2"}, {ID: "c3"}}
	m.selectedIdx = 1 // select b2

	// update with re-ordered list, b2 now at index 2
	updated := []types.ClientInfo{{ID: "a1"}, {ID: "c3"}, {ID: "b2"}}
	// craft internal message type
	msg := clientsMsg(updated)
	_, _ = m.Update(msg)

	if m.selectedIdx != 2 {
		t.Fatalf("expected selection moved to index 2, got %d", m.selectedIdx)
	}
}

// test resetting dropped logs via 'R' in server logs view
func TestResetDroppedLogs(t *testing.T) {
	m := &TUIModel{viewMode: ViewServerLogs, droppedLogs: 10}
	_, _ = m.handleServerLogsKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	if m.droppedLogs != 0 {
		t.Fatalf("expected droppedLogs reset to 0, got %d", m.droppedLogs)
	}
}
