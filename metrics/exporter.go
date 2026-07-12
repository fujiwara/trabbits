package metrics

import (
	"cmp"
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// newReaderFromEnv creates a metric reader configured by standard OTEL_*
// environment variables.
//
// OTEL_METRICS_EXPORTER selects the exporter: "none", "console" or "otlp".
// When unset, it defaults to "otlp" if OTEL_EXPORTER_OTLP_ENDPOINT or
// OTEL_EXPORTER_OTLP_METRICS_ENDPOINT is set, otherwise "none"
// (returns a nil reader; nothing is exported).
//
// The OTLP protocol is selected by OTEL_EXPORTER_OTLP_METRICS_PROTOCOL or
// OTEL_EXPORTER_OTLP_PROTOCOL ("grpc" or "http/protobuf", default
// "http/protobuf"). Endpoint, headers, TLS and timeout are handled by the
// exporters themselves via their standard environment variables, and the
// periodic reader honors OTEL_METRIC_EXPORT_INTERVAL / _TIMEOUT.
func newReaderFromEnv(ctx context.Context) (sdkmetric.Reader, error) {
	name := os.Getenv("OTEL_METRICS_EXPORTER")
	if name == "" {
		if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
			os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT") != "" {
			name = "otlp"
		} else {
			name = "none"
		}
	}
	switch name {
	case "none":
		return nil, nil
	case "console":
		exp, err := stdoutmetric.New()
		if err != nil {
			return nil, fmt.Errorf("failed to create console metrics exporter: %w", err)
		}
		return sdkmetric.NewPeriodicReader(exp), nil
	case "otlp":
		proto := cmp.Or(
			os.Getenv("OTEL_EXPORTER_OTLP_METRICS_PROTOCOL"),
			os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL"),
			"http/protobuf",
		)
		var exp sdkmetric.Exporter
		var err error
		switch proto {
		case "grpc":
			exp, err = otlpmetricgrpc.New(ctx)
		case "http/protobuf":
			exp, err = otlpmetrichttp.New(ctx)
		default:
			return nil, fmt.Errorf("unsupported OTLP protocol: %q", proto)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP metrics exporter: %w", err)
		}
		return sdkmetric.NewPeriodicReader(exp), nil
	default:
		return nil, fmt.Errorf("unsupported OTEL_METRICS_EXPORTER: %q", name)
	}
}
