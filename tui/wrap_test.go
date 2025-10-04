package tui

import "testing"

func TestWrapTextASCII(t *testing.T) {
	text := "the quick brown fox jumps over the lazy dog"
	lines := wrapText(text, 10)
	if len(lines) < 4 {
		t.Fatalf("expected multiple lines, got %v", lines)
	}
	for i, ln := range lines {
		if len(ln) > 10 {
			t.Errorf("line %d too long: %q", i, ln)
		}
	}
}

func TestWrapTextWideRunes(t *testing.T) {
	// Japanese wide characters should count as width 2
	text := "あいうえおabc"
	// width 6 should allow up to 3 wide runes per line
	lines := wrapText(text, 6)
	if len(lines) == 0 {
		t.Fatal("wrap produced no lines")
	}
	// Ensure we did wrap (likely more than 1 line)
	if len(lines) == 1 {
		t.Errorf("expected wrapping with width 6, got single line: %v", lines)
	}
}
