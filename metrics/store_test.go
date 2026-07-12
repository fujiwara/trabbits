package metrics_test

import (
	"testing"

	"github.com/fujiwara/trabbits/metrics"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestMetricsExport(t *testing.T) {
	ctx := t.Context()
	reader := sdkmetric.NewManualReader()
	store, err := metrics.NewStore(ctx,
		metrics.WithReader(reader),
		metrics.WithServiceVersion("test-version"),
		metrics.WithAttributes(map[string]string{"env": "test"}),
	)
	if err != nil {
		t.Fatalf("failed to create metrics store: %v", err)
	}
	defer store.Shutdown(ctx)

	m := store.Metrics()
	m.ClientConnections.Inc()
	m.ClientConnections.Inc()
	m.ClientConnections.Dec()
	m.ClientTotalConnections.Inc()
	m.ClientTotalConnections.Inc()
	m.UpstreamConnections.WithLabelValues("localhost:5672").Inc()
	m.ProcessedMessages.WithLabelValues("Basic.Publish").Inc()
	m.SetHealthyNodes("cluster1", 2)
	m.SetUnhealthyNodes("cluster1", 1)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("failed to collect metrics: %v", err)
	}

	// Verify resource attributes
	res := rm.Resource.Set()
	for key, want := range map[string]string{
		"service.name":    "trabbits",
		"service.version": "test-version",
		"env":             "test",
	} {
		if v, ok := res.Value(attribute.Key(key)); !ok || v.AsString() != want {
			t.Errorf("resource attribute %s = %q, want %q", key, v.AsString(), want)
		}
	}

	// Verify exported metric values
	tests := []struct {
		name  string
		attrs []attribute.KeyValue
		want  int64
	}{
		{"trabbits_client_connections", nil, 1},
		{"trabbits_client_connections_total", nil, 2},
		{"trabbits_upstream_connections", []attribute.KeyValue{attribute.String("addr", "localhost:5672")}, 1},
		{"trabbits_processed_messages_total", []attribute.KeyValue{attribute.String("method", "Basic.Publish")}, 1},
		{"trabbits_upstream_healthy_nodes", []attribute.KeyValue{attribute.String("upstream", "cluster1")}, 2},
		{"trabbits_upstream_unhealthy_nodes", []attribute.KeyValue{attribute.String("upstream", "cluster1")}, 1},
	}
	for _, tt := range tests {
		md, ok := findMetric(rm, tt.name)
		if !ok {
			t.Errorf("metric %s not found", tt.name)
			continue
		}
		got, ok := dataPointValue(md, attribute.NewSet(tt.attrs...))
		if !ok {
			t.Errorf("metric %s has no data point with attributes %v", tt.name, tt.attrs)
			continue
		}
		if got != tt.want {
			t.Errorf("metric %s = %d, want %d", tt.name, got, tt.want)
		}
	}

	// Counters must be monotonic sums, up/down counters must not be
	for name, monotonic := range map[string]bool{
		"trabbits_client_connections_total": true,
		"trabbits_client_connections":       false,
	} {
		md, ok := findMetric(rm, name)
		if !ok {
			t.Errorf("metric %s not found", name)
			continue
		}
		sum, ok := md.Data.(metricdata.Sum[int64])
		if !ok {
			t.Errorf("metric %s is not a sum: %T", name, md.Data)
			continue
		}
		if sum.IsMonotonic != monotonic {
			t.Errorf("metric %s monotonic = %v, want %v", name, sum.IsMonotonic, monotonic)
		}
	}
}

func findMetric(rm metricdata.ResourceMetrics, name string) (metricdata.Metrics, bool) {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return m, true
			}
		}
	}
	return metricdata.Metrics{}, false
}

func dataPointValue(md metricdata.Metrics, attrs attribute.Set) (int64, bool) {
	switch data := md.Data.(type) {
	case metricdata.Sum[int64]:
		for _, dp := range data.DataPoints {
			if dp.Attributes.Equals(&attrs) {
				return dp.Value, true
			}
		}
	case metricdata.Gauge[int64]:
		for _, dp := range data.DataPoints {
			if dp.Attributes.Equals(&attrs) {
				return dp.Value, true
			}
		}
	}
	return 0, false
}
