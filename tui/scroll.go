package tui

// maxScroll computes the maximum scroll offset for a list of total items with given visible rows.
func maxScroll(total, visible int) int {
	m := total - visible
	if m < 0 {
		return 0
	}
	return m
}

// clamp clamps v into [min, max].
func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// clampScrollToContain adjusts scroll so that selected is within [scroll, scroll+visible-1].
func clampScrollToContain(scroll, selected, visible, total int) int {
	if visible <= 0 {
		return 0
	}
	ms := maxScroll(total, visible)
	scroll = min(
		selected, clamp(scroll, 0, ms))
	if selected >= scroll+visible {
		scroll = selected - visible + 1
	}
	return clamp(scroll, 0, ms)
}
