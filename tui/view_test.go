package tui

import (
	"fmt"
	"strings"
	"testing"
)

// TestRenderLogsWithWrapping tests the common log rendering logic
func TestRenderLogsWithWrapping(t *testing.T) {
	tests := []struct {
		name            string
		logCount        int
		maxDisplayLines int
		scrollPos       int
		selectedIdx     int
		logLines        []string // each entry can be multi-line
		wantStartIdx    int
		wantEndIdx      int
		wantSelected    int // index in renderedLines that should be selected (-1 if none)
	}{
		{
			name:            "simple case - all fit",
			logCount:        3,
			maxDisplayLines: 10,
			scrollPos:       0,
			selectedIdx:     1,
			logLines:        []string{"log 0", "log 1", "log 2"},
			wantStartIdx:    0,
			wantEndIdx:      3,
			wantSelected:    1,
		},
		{
			name:            "selected entry forced visible from beginning",
			logCount:        5,
			maxDisplayLines: 3,
			scrollPos:       0,
			selectedIdx:     4,
			logLines:        []string{"log 0", "log 1", "log 2", "log 3", "log 4"},
			wantStartIdx:    2, // should center around selected
			wantEndIdx:      5,
			wantSelected:    2, // selected is at index 2 in rendered output
		},
		{
			name:            "wrapped logs - selected visible",
			logCount:        3,
			maxDisplayLines: 5,
			scrollPos:       0,
			selectedIdx:     1,
			logLines: []string{
				"short",
				"line1\nline2\nline3", // 3 lines
				"another",
			},
			wantStartIdx: 0,
			wantEndIdx:   3,
			wantSelected: 1, // all should fit
		},
		{
			name:            "wrapped logs exceed space - selected still visible",
			logCount:        3,
			maxDisplayLines: 4,
			scrollPos:       0,
			selectedIdx:     2, // select the last one
			logLines: []string{
				"short",
				"line1\nline2\nline3", // 3 lines - would exceed if included
				"selected",            // 1 line
			},
			wantStartIdx: 1, // should skip first to show selected
			wantEndIdx:   3,
			wantSelected: 1,
		},
		{
			name:            "selected before scrollPos",
			logCount:        10,
			maxDisplayLines: 5,
			scrollPos:       5,
			selectedIdx:     2,
			logLines:        makeSimpleLogs(10),
			wantStartIdx:    2, // should adjust to show selected
			wantEndIdx:      7,
			wantSelected:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatFunc := func(idx int, isSelected bool) string {
				line := tt.logLines[idx]
				if isSelected {
					return "[SELECTED] " + line
				}
				return line
			}

			result := renderLogsWithWrapping(
				tt.logCount,
				tt.maxDisplayLines,
				tt.scrollPos,
				tt.selectedIdx,
				formatFunc,
			)

			if result.startIdx != tt.wantStartIdx {
				t.Errorf("startIdx = %d, want %d", result.startIdx, tt.wantStartIdx)
			}
			if result.endIdx != tt.wantEndIdx {
				t.Errorf("endIdx = %d, want %d", result.endIdx, tt.wantEndIdx)
			}

			// Check that selected entry is present and marked
			if tt.wantSelected >= 0 {
				if tt.wantSelected >= len(result.renderedLines) {
					t.Fatalf("wantSelected %d >= len(renderedLines) %d", tt.wantSelected, len(result.renderedLines))
				}
				selectedLine := result.renderedLines[tt.wantSelected]
				if !strings.Contains(selectedLine, "[SELECTED]") {
					t.Errorf("renderedLines[%d] should contain [SELECTED], got: %s", tt.wantSelected, selectedLine)
				}
			}

			// Check that total display lines don't exceed maxDisplayLines
			totalLines := 0
			for _, line := range result.renderedLines {
				totalLines += strings.Count(line, "\n") + 1
			}
			if totalLines > tt.maxDisplayLines {
				t.Errorf("totalLines = %d, exceeds maxDisplayLines = %d", totalLines, tt.maxDisplayLines)
			}
		})
	}
}

func makeSimpleLogs(count int) []string {
	logs := make([]string, count)
	for i := 0; i < count; i++ {
		logs[i] = fmt.Sprintf("log %d", i)
	}
	return logs
}

func TestRenderLogsWithWrapping_EmptyLogs(t *testing.T) {
	result := renderLogsWithWrapping(0, 10, 0, 0, func(idx int, isSelected bool) string {
		return "should not be called"
	})

	if len(result.renderedLines) != 0 {
		t.Errorf("expected no rendered lines for empty logs, got %d", len(result.renderedLines))
	}
}

func TestRenderLogsWithWrapping_SingleLongEntry(t *testing.T) {
	// A single log entry that spans multiple lines
	longLog := strings.Repeat("x", 100) + "\n" + strings.Repeat("y", 100) + "\n" + strings.Repeat("z", 100)

	formatFunc := func(idx int, isSelected bool) string {
		if isSelected {
			return "[SEL] " + longLog
		}
		return longLog
	}

	result := renderLogsWithWrapping(1, 5, 0, 0, formatFunc)

	if len(result.renderedLines) != 1 {
		t.Errorf("expected 1 rendered line, got %d", len(result.renderedLines))
	}

	// Even if it exceeds maxDisplayLines, the selected entry should be shown
	if !strings.Contains(result.renderedLines[0], "[SEL]") {
		t.Errorf("selected entry should be marked, got: %s", result.renderedLines[0])
	}
}

func TestRenderLogsWithWrapping_SelectedInMiddle(t *testing.T) {
	// 10 logs, select middle one, limited display space
	logCount := 10
	maxDisplayLines := 5
	selectedIdx := 5

	formatFunc := func(idx int, isSelected bool) string {
		if isSelected {
			return fmt.Sprintf("[SEL] log %d", idx)
		}
		return fmt.Sprintf("log %d", idx)
	}

	result := renderLogsWithWrapping(logCount, maxDisplayLines, 0, selectedIdx, formatFunc)

	// Selected should be visible
	selectedFound := false
	for _, line := range result.renderedLines {
		if strings.Contains(line, "[SEL]") {
			selectedFound = true
			break
		}
	}

	if !selectedFound {
		t.Errorf("selected entry not found in rendered output")
	}

	// Should render entries around the selected one
	if result.startIdx > selectedIdx {
		t.Errorf("startIdx %d should be <= selectedIdx %d", result.startIdx, selectedIdx)
	}
	if result.endIdx <= selectedIdx {
		t.Errorf("endIdx %d should be > selectedIdx %d", result.endIdx, selectedIdx)
	}
}
