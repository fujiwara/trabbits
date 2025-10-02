package tui

import (
	"testing"
	"time"
)

func TestFormatID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "short ID",
			input:    "abc123",
			expected: "abc123",
		},
		{
			name:     "exact 8 chars",
			input:    "abcd1234",
			expected: "abcd1234",
		},
		{
			name:     "exact 10 chars",
			input:    "abcd123456",
			expected: "abcd123456",
		},
		{
			name:     "long ID",
			input:    "abcdefghijklmnop",
			expected: "abcdefgh..",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatID(tt.input)
			if result != tt.expected {
				t.Errorf("formatID(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "seconds",
			duration: 30 * time.Second,
			expected: "30s ago",
		},
		{
			name:     "minutes",
			duration: 2 * time.Minute,
			expected: "2m ago",
		},
		{
			name:     "hours",
			duration: 3 * time.Hour,
			expected: "3h ago",
		},
		{
			name:     "days",
			duration: 25 * time.Hour,
			expected: "1d ago",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, result, tt.expected)
			}
		})
	}
}

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected string
	}{
		{
			name:     "small number",
			input:    123,
			expected: "123",
		},
		{
			name:     "thousands",
			input:    1500,
			expected: "1.5K",
		},
		{
			name:     "millions",
			input:    2500000,
			expected: "2.5M",
		},
		{
			name:     "billions",
			input:    3500000000,
			expected: "3.5G",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatNumber(tt.input)
			if result != tt.expected {
				t.Errorf("formatNumber(%d) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "short string",
			input:    "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "exact length",
			input:    "hello",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "long string",
			input:    "hello world",
			maxLen:   8,
			expected: "hello ..",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncate(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
			}
		})
	}
}
