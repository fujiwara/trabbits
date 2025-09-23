package trabbits_test

import (
	"strings"
	"testing"

	"github.com/fujiwara/trabbits"
)

func TestColoredDiff(t *testing.T) {
	// Test the coloredDiff function with simple input/output
	input := `- removed line
+ added line
  unchanged line
- another removed line
+ another added line`

	result := trabbits.ColoredDiff(input)

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
	client := trabbits.NewAPIClient(socketPath)

	if client == nil {
		t.Fatal("newAPIClient should not return nil")
	}

	// Since endpoint is unexported, we just verify the client is properly created
	// The actual endpoint value is verified through functional tests
}
