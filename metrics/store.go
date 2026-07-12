package metrics

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
)

const meterName = "github.com/fujiwara/trabbits"

// Store manages metrics and their OpenTelemetry meter provider for a server instance
type Store struct {
	metrics  *Metrics
	provider *sdkmetric.MeterProvider
}

type options struct {
	reader         sdkmetric.Reader
	attributes     map[string]string
	serviceVersion string
}

// Option configures a Store.
type Option func(*options)

// WithReader sets an explicit metric reader instead of the one
// configured by OTEL_* environment variables. Useful for testing.
func WithReader(r sdkmetric.Reader) Option {
	return func(o *options) { o.reader = r }
}

// WithAttributes adds custom resource attributes.
func WithAttributes(attrs map[string]string) Option {
	return func(o *options) { o.attributes = attrs }
}

// WithServiceVersion sets the service.version resource attribute.
func WithServiceVersion(v string) Option {
	return func(o *options) { o.serviceVersion = v }
}

// NewStore creates a new metrics store. Unless WithReader is given, the
// exporter is configured from standard OTEL_* environment variables
// (see newReaderFromEnv); when none are set, metrics are collected in
// memory only and nothing is exported.
func NewStore(ctx context.Context, opts ...Option) (*Store, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	reader := o.reader
	if reader == nil {
		var err error
		reader, err = newReaderFromEnv(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to configure metrics exporter: %w", err)
		}
	}

	custom := make([]attribute.KeyValue, 0, len(o.attributes))
	for k, v := range o.attributes {
		custom = append(custom, attribute.String(k, v))
	}
	// Environment resource attributes (OTEL_SERVICE_NAME, OTEL_RESOURCE_ATTRIBUTES)
	// and custom attributes from the config override the defaults.
	defaults := []attribute.KeyValue{semconv.ServiceName("trabbits")}
	if o.serviceVersion != "" {
		defaults = append(defaults, semconv.ServiceVersion(o.serviceVersion))
	}
	envRes, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(custom...),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}
	res, err := resource.Merge(resource.NewSchemaless(defaults...), envRes)
	if err != nil {
		return nil, fmt.Errorf("failed to merge resources: %w", err)
	}

	pOpts := []sdkmetric.Option{sdkmetric.WithResource(res)}
	if reader != nil {
		pOpts = append(pOpts, sdkmetric.WithReader(reader))
	}
	provider := sdkmetric.NewMeterProvider(pOpts...)

	m, err := NewMetrics(provider.Meter(meterName))
	if err != nil {
		provider.Shutdown(ctx)
		return nil, fmt.Errorf("failed to create metrics: %w", err)
	}
	return &Store{metrics: m, provider: provider}, nil
}

// NewTestStore creates a metrics store without any exporter. Metrics are
// still collected in memory. It is used as the default store for servers
// created without WithMetricsStore (e.g. in tests).
func NewTestStore() *Store {
	provider := sdkmetric.NewMeterProvider()
	m, err := NewMetrics(provider.Meter(meterName))
	if err != nil {
		// registration on a fresh provider cannot fail
		panic(fmt.Sprintf("failed to create metrics: %v", err))
	}
	return &Store{metrics: m, provider: provider}
}

// Metrics returns the metrics instance for this store
func (s *Store) Metrics() *Metrics {
	return s.metrics
}

// Shutdown flushes remaining metrics and shuts down the meter provider.
func (s *Store) Shutdown(ctx context.Context) error {
	return s.provider.Shutdown(ctx)
}
