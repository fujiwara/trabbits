package metrics_test

import (
	"testing"

	"github.com/fujiwara/trabbits/metrics"
)

func TestNewReaderFromEnv(t *testing.T) {
	envKeys := []string{
		"OTEL_METRICS_EXPORTER",
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
		"OTEL_EXPORTER_OTLP_PROTOCOL",
		"OTEL_EXPORTER_OTLP_METRICS_PROTOCOL",
	}
	tests := []struct {
		name       string
		env        map[string]string
		wantReader bool
		wantErr    bool
	}{
		{
			name:       "no env vars: no reader",
			env:        nil,
			wantReader: false,
		},
		{
			name:       "exporter none",
			env:        map[string]string{"OTEL_METRICS_EXPORTER": "none"},
			wantReader: false,
		},
		{
			name:       "exporter console",
			env:        map[string]string{"OTEL_METRICS_EXPORTER": "console"},
			wantReader: true,
		},
		{
			name:       "otlp endpoint set: defaults to otlp",
			env:        map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "http://localhost:4318"},
			wantReader: true,
		},
		{
			name:       "otlp metrics endpoint set: defaults to otlp",
			env:        map[string]string{"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT": "http://localhost:4318/v1/metrics"},
			wantReader: true,
		},
		{
			name: "otlp with grpc protocol",
			env: map[string]string{
				"OTEL_METRICS_EXPORTER":       "otlp",
				"OTEL_EXPORTER_OTLP_PROTOCOL": "grpc",
			},
			wantReader: true,
		},
		{
			name: "otlp with unsupported protocol",
			env: map[string]string{
				"OTEL_METRICS_EXPORTER":       "otlp",
				"OTEL_EXPORTER_OTLP_PROTOCOL": "http/json",
			},
			wantErr: true,
		},
		{
			name:    "unsupported exporter",
			env:     map[string]string{"OTEL_METRICS_EXPORTER": "prometheus"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range envKeys {
				t.Setenv(k, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			reader, err := metrics.NewReaderFromEnv(t.Context())
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := reader != nil; got != tt.wantReader {
				t.Errorf("reader = %v, want reader %v", reader, tt.wantReader)
			}
			if reader != nil {
				if err := reader.Shutdown(t.Context()); err != nil {
					t.Errorf("failed to shutdown reader: %v", err)
				}
			}
		})
	}
}
