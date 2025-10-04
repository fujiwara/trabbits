package tui

import "testing"

func TestMaxScroll(t *testing.T) {
	cases := []struct{ total, visible, want int }{
		{0, 0, 0},
		{0, 5, 0},
		{5, 10, 0},
		{10, 5, 5},
		{10, 1, 9},
	}
	for _, c := range cases {
		if got := maxScroll(c.total, c.visible); got != c.want {
			t.Errorf("maxScroll(%d,%d)=%d want %d", c.total, c.visible, got, c.want)
		}
	}
}

func TestClamp(t *testing.T) {
	if clamp(5, 0, 10) != 5 {
		t.Errorf("clamp normal")
	}
	if clamp(-1, 0, 10) != 0 {
		t.Errorf("clamp low")
	}
	if clamp(11, 0, 10) != 10 {
		t.Errorf("clamp high")
	}
}

func TestClampScrollToContain(t *testing.T) {
	total, visible := 100, 10
	// selected inside window: no change
	if got := clampScrollToContain(20, 25, visible, total); got != 20 {
		t.Errorf("contain inside: got %d want 20", got)
	}
	// selected above window: scroll up
	if got := clampScrollToContain(20, 18, visible, total); got != 18 {
		t.Errorf("contain above: got %d want 18", got)
	}
	// selected below window: scroll down so it appears at bottom
	if got := clampScrollToContain(20, 31, visible, total); got != 22 {
		t.Errorf("contain below: got %d want 22", got)
	}
	// clamp at bounds when near end
	if got := clampScrollToContain(95, 99, visible, total); got != 90 {
		t.Errorf("contain end: got %d want 90", got)
	}
}
