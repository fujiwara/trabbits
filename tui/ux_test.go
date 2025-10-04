package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/fujiwara/trabbits/types"
	"os"
	"testing"
	"time"
)

func TestProbeAutoScrollBehavior(t *testing.T) {
	m := &TUIModel{viewMode: ViewProbe, height: 20}
	m.probeState = &probeState{clientID: "p1", autoScroll: true}

	// send one probe log
	entry := probeLogEntry{Timestamp: time.Now(), Message: "log1"}
	_, _ = m.Update(probeLogMsg(entry))

	if m.probeState.selectedIdx != 0 {
		t.Fatalf("expected selectedIdx=0, got %d", m.probeState.selectedIdx)
	}
	// with autoScroll on, scroll should be bottom (maxScroll)
	vis := m.getProbeVisibleRows()
	wantScroll := maxScroll(len(m.probeState.logs), vis)
	if m.probeState.scroll != wantScroll {
		t.Fatalf("expected scroll %d, got %d", wantScroll, m.probeState.scroll)
	}

	// turn off autoScroll and add more logs, scroll should not jump to bottom
	m.probeState.autoScroll = false
	m.probeState.scroll = 0
	_, _ = m.Update(probeLogMsg(probeLogEntry{Timestamp: time.Now(), Message: "log2"}))
	if m.probeState.scroll != 0 {
		t.Fatalf("expected scroll remain 0 when autoScroll off, got %d", m.probeState.scroll)
	}
}

func TestListVimNavigation(t *testing.T) {
	m := &TUIModel{viewMode: ViewList, height: 20}
	m.clients = []types.ClientInfo{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	m.selectedIdx = 1
	// g -> top
	_, _ = m.handleListKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if m.selectedIdx != 0 {
		t.Fatalf("expected top, got %d", m.selectedIdx)
	}
	// G -> bottom
	_, _ = m.handleListKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	if m.selectedIdx != 2 {
		t.Fatalf("expected bottom index 2, got %d", m.selectedIdx)
	}
}

func TestServerLogsNavAndReset(t *testing.T) {
	m := &TUIModel{viewMode: ViewServerLogs, height: 20}
	// populate some logs
	for i := 0; i < 5; i++ {
		_, _ = m.Update(logMsg(LogEntry{Time: time.Now(), Message: "log"}))
	}
	// g to top
	_, _ = m.handleServerLogsKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if m.serverLogsSelectedIdx != 0 {
		t.Fatalf("expected top")
	}
	// G to bottom
	_, _ = m.handleServerLogsKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	if want := len(m.logEntries) - 1; m.serverLogsSelectedIdx != want {
		t.Fatalf("expected bottom %d, got %d", want, m.serverLogsSelectedIdx)
	}
	// reset dropped logs
	m.droppedLogs = 3
	_, _ = m.handleServerLogsKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	if m.droppedLogs != 0 {
		t.Fatalf("expected droppedLogs reset to 0")
	}
}

func TestSaveOverwriteConfirmFlow(t *testing.T) {
	// Create a temp file to trigger overwrite flow
	f, err := os.CreateTemp("", "tui-overwrite-*.log")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.Close()

	m := &TUIModel{viewMode: ViewSaveConfirm, height: 20}
	// probe logs for saving
	m.probeState = &probeState{clientID: "p1", logs: []probeLogEntry{{Timestamp: time.Now(), Message: "x"}}}
	m.saveState = &saveState{clientID: "p1", filePath: f.Name(), previousView: ViewProbe}

	// First ENTER should set overwriteConfirm=true and not save yet
	_, _ = m.handleSaveConfirmKeys(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.saveState.overwriteConfirm {
		t.Fatalf("expected overwriteConfirm=true after first ENTER")
	}

	// Second ENTER should perform save via command
	_, cmd := m.handleSaveConfirmKeys(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected save command on second ENTER")
	}
	// Execute command
	msg := cmd()
	if _, ok := msg.(successMsg); !ok {
		t.Fatalf("expected successMsg after save, got %#v", msg)
	}
}
