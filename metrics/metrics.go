package metrics

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metrics contains all metrics for trabbits.
// The values are held in memory and observed by OpenTelemetry observable
// instruments at collection time. This allows call sites to update metrics
// without a context.Context and to read current values back
// (e.g. for least connection balancing).
type Metrics struct {
	ClientConnections      *Gauge
	ClientTotalConnections *Counter
	ClientConnectionErrors *Counter

	ClientReceivedFrames *Counter
	ClientSentFrames     *Counter

	UpstreamConnections      *GaugeVec
	UpstreamTotalConnections *CounterVec
	UpstreamConnectionErrors *CounterVec

	UpstreamHealthyNodes   *GaugeVec
	UpstreamUnhealthyNodes *GaugeVec

	ProcessedMessages *CounterVec
	ErroredMessages   *CounterVec

	LoggerStats *CounterVec

	PanicRecoveries *CounterVec
}

// NewMetrics creates a new Metrics instance and registers observable
// instruments on the given meter.
func NewMetrics(meter metric.Meter) (*Metrics, error) {
	m := &Metrics{
		ClientConnections:      &Gauge{},
		ClientTotalConnections: &Counter{},
		ClientConnectionErrors: &Counter{},

		ClientReceivedFrames: &Counter{},
		ClientSentFrames:     &Counter{},

		UpstreamConnections:      newGaugeVec(),
		UpstreamTotalConnections: newCounterVec(),
		UpstreamConnectionErrors: newCounterVec(),

		UpstreamHealthyNodes:   newGaugeVec(),
		UpstreamUnhealthyNodes: newGaugeVec(),

		ProcessedMessages: newCounterVec(),
		ErroredMessages:   newCounterVec(),

		LoggerStats: newCounterVec(),

		PanicRecoveries: newCounterVec(),
	}

	var errs []error
	counter := func(name, desc string) metric.Int64ObservableCounter {
		c, err := meter.Int64ObservableCounter(name, metric.WithDescription(desc))
		errs = append(errs, err)
		return c
	}
	upDownCounter := func(name, desc string) metric.Int64ObservableUpDownCounter {
		c, err := meter.Int64ObservableUpDownCounter(name, metric.WithDescription(desc))
		errs = append(errs, err)
		return c
	}
	gauge := func(name, desc string) metric.Int64ObservableGauge {
		g, err := meter.Int64ObservableGauge(name, metric.WithDescription(desc))
		errs = append(errs, err)
		return g
	}

	clientConnections := upDownCounter("trabbits_client_connections",
		"Number of client connections.")
	clientTotalConnections := counter("trabbits_client_connections_total",
		"Total number of client connections.")
	clientConnectionErrors := counter("trabbits_client_connection_errors_total",
		"Number of client connection errors.")

	clientReceivedFrames := counter("trabbits_client_received_frames_total",
		"Number of received frames from clients.")
	clientSentFrames := counter("trabbits_client_sent_frames_total",
		"Number of sent frames to clients.")

	upstreamConnections := upDownCounter("trabbits_upstream_connections",
		"Number of upstream connections.")
	upstreamTotalConnections := counter("trabbits_upstream_connections_total",
		"Total number of upstream connections.")
	upstreamConnectionErrors := counter("trabbits_upstream_connection_errors_total",
		"Number of upstream connection errors.")

	upstreamHealthyNodes := gauge("trabbits_upstream_healthy_nodes",
		"Number of healthy nodes in upstream cluster.")
	upstreamUnhealthyNodes := gauge("trabbits_upstream_unhealthy_nodes",
		"Number of unhealthy nodes in upstream cluster.")

	processedMessages := counter("trabbits_processed_messages_total",
		"Number of processed messages by method.")
	erroredMessages := counter("trabbits_errored_messages_total",
		"Number of errored messages by method.")

	loggerStats := counter("trabbits_logger_stats_total",
		"Number of logger stats by level.")

	panicRecoveries := counter("trabbits_panic_recoveries_total",
		"Number of panic recoveries by function.")

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	observeVec := func(o metric.Observer, inst metric.Int64Observable, key string) func(string, int64) {
		return func(lv string, v int64) {
			o.ObserveInt64(inst, v, metric.WithAttributes(attribute.String(key, lv)))
		}
	}
	_, err := meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		o.ObserveInt64(clientConnections, m.ClientConnections.Value())
		o.ObserveInt64(clientTotalConnections, m.ClientTotalConnections.Value())
		o.ObserveInt64(clientConnectionErrors, m.ClientConnectionErrors.Value())

		o.ObserveInt64(clientReceivedFrames, m.ClientReceivedFrames.Value())
		o.ObserveInt64(clientSentFrames, m.ClientSentFrames.Value())

		m.UpstreamConnections.each(observeVec(o, upstreamConnections, "addr"))
		m.UpstreamTotalConnections.each(observeVec(o, upstreamTotalConnections, "addr"))
		m.UpstreamConnectionErrors.each(observeVec(o, upstreamConnectionErrors, "addr"))

		m.UpstreamHealthyNodes.each(observeVec(o, upstreamHealthyNodes, "upstream"))
		m.UpstreamUnhealthyNodes.each(observeVec(o, upstreamUnhealthyNodes, "upstream"))

		m.ProcessedMessages.each(observeVec(o, processedMessages, "method"))
		m.ErroredMessages.each(observeVec(o, erroredMessages, "method"))

		m.LoggerStats.each(observeVec(o, loggerStats, "level"))

		m.PanicRecoveries.each(observeVec(o, panicRecoveries, "function"))
		return nil
	},
		clientConnections,
		clientTotalConnections,
		clientConnectionErrors,
		clientReceivedFrames,
		clientSentFrames,
		upstreamConnections,
		upstreamTotalConnections,
		upstreamConnectionErrors,
		upstreamHealthyNodes,
		upstreamUnhealthyNodes,
		processedMessages,
		erroredMessages,
		loggerStats,
		panicRecoveries,
	)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// SetHealthyNodes sets the number of healthy nodes for an upstream
func (m *Metrics) SetHealthyNodes(upstream string, count int) {
	m.UpstreamHealthyNodes.WithLabelValues(upstream).Set(int64(count))
}

// SetUnhealthyNodes sets the number of unhealthy nodes for an upstream
func (m *Metrics) SetUnhealthyNodes(upstream string, count int) {
	m.UpstreamUnhealthyNodes.WithLabelValues(upstream).Set(int64(count))
}
