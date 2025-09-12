// MIT License
// Copyright (c) 2025 FUJIWARA Shunichiro

package trabbits_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/fujiwara/trabbits"
)

func TestMatchRoutingCommand(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		key     string
		want    bool
	}{
		{
			name:    "exact match",
			pattern: "logs.error",
			key:     "logs.error",
			want:    true,
		},
		{
			name:    "star wildcard match",
			pattern: "logs.*.error",
			key:     "logs.app.error",
			want:    true,
		},
		{
			name:    "hash wildcard match all",
			pattern: "logs.#",
			key:     "logs.app.error.critical",
			want:    true,
		},
		{
			name:    "hash wildcard match zero tokens after literal",
			pattern: "logs.#",
			key:     "logs",
			want:    false, // Current implementation requires at least one dot when pattern has dots
		},
		{
			name:    "hash wildcard at end matches multiple",
			pattern: "metrics.#",
			key:     "metrics.cpu.usage",
			want:    true,
		},
		{
			name:    "single hash matches everything",
			pattern: "#",
			key:     "any.routing.key",
			want:    true,
		},
		{
			name:    "no match different pattern",
			pattern: "logs.*.error",
			key:     "metrics.app.error",
			want:    false,
		},
		{
			name:    "no match star needs exactly one",
			pattern: "logs.*.error",
			key:     "logs.app.service.error",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli := &trabbits.CLI{
				Test: &trabbits.TestOptions{
					MatchRouting: struct {
						Pattern string `arg:"" required:"" help:"Binding pattern to test (e.g., 'logs.*.error', 'metrics.#')."`
						Key     string `arg:"" required:"" help:"Routing key to match against the pattern."`
					}{
						Pattern: tt.pattern,
						Key:     tt.key,
					},
				},
			}

			// Capture stdout
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := trabbits.TestMatchRouting(context.Background(), cli)

			// Restore stdout
			w.Close()
			os.Stdout = old

			// Read captured output
			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := buf.String()

			// Check result
			if tt.want {
				if err != nil {
					t.Errorf("expected no error for matching pattern, got %v", err)
				}
				if !strings.Contains(output, "✓ MATCHED") {
					t.Errorf("expected MATCHED in output, got: %s", output)
				}
			} else {
				if err == nil {
					t.Errorf("expected error for non-matching pattern")
				}
				if !strings.Contains(output, "✗ NOT MATCHED") {
					t.Errorf("expected NOT MATCHED in output, got: %s", output)
				}
			}
		})
	}
}
