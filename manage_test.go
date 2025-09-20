package trabbits

import (
	"strings"
	"testing"
)

func TestColoredDiff(t *testing.T) {
	// Test the coloredDiff function with simple input/output
	input := `- removed line
+ added line
  unchanged line
- another removed line
+ another added line`

	result := coloredDiff(input)

	// Check that the result contains the input (colors are applied but content preserved)
	if !strings.Contains(result, "removed line") {
		t.Error("coloredDiff should preserve removed lines")
	}
	if !strings.Contains(result, "added line") {
		t.Error("coloredDiff should preserve added lines")
	}
	if !strings.Contains(result, "unchanged line") {
		t.Error("coloredDiff should preserve unchanged lines")
	}
}

func TestNewAPIClient(t *testing.T) {
	// Test the newAPIClient function
	socketPath := "/tmp/test.sock"
	client := newAPIClient(socketPath)

	if client == nil {
		t.Fatal("newAPIClient should not return nil")
	}

	if client.endpoint != "http://localhost/" {
		t.Errorf("Expected endpoint to be 'http://localhost/', got %s", client.endpoint)
	}
}
