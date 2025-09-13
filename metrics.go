package trabbits

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var metrics *Metrics
var metricsReg *prometheus.Registry
var metricsOnce sync.Once

func initMetrics() {
	metricsOnce.Do(func() {
		metrics = NewMetrics()
		metricsReg = prometheus.NewRegistry()
		metrics.MustRegister(metricsReg)
	})
}

func GetMetrics() *Metrics {
	initMetrics()
	return metrics
}

type Metrics struct {
	ClientConnections      prometheus.Gauge
	ClientTotalConnections prometheus.Counter
	ClientConnectionErrors prometheus.Counter

	ClientReceivedFrames prometheus.Counter
	ClientSentFrames     prometheus.Counter

	UpstreamConnections      *prometheus.GaugeVec
	UpstreamTotalConnections *prometheus.CounterVec
	UpstreamConnectionErrors *prometheus.CounterVec

	UpstreamHealthyNodes   *prometheus.GaugeVec
	UpstreamUnhealthyNodes *prometheus.GaugeVec

	ProcessedMessages *prometheus.CounterVec
	ErroredMessages   *prometheus.CounterVec

	LoggerStats *prometheus.CounterVec

	PanicRecoveries *prometheus.CounterVec
}

func NewMetrics() *Metrics {
	return &Metrics{
		ClientConnections: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "trabbits_client_connections",
			Help: "Number of client connections.",
		}),
		ClientTotalConnections: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "trabbits_client_connections_total",
			Help: "Total number of client connections.",
		}),
		ClientConnectionErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "trabbits_client_connection_errors_total",
			Help: "Number of client connection errors.",
		}),

		ClientReceivedFrames: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "trabbits_client_received_frames_total",
			Help: "Number of received frames from clients.",
		}),
		ClientSentFrames: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "trabbits_client_sent_frames_total",
			Help: "Number of sent frames to clients.",
		}),

		UpstreamConnections: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "trabbits_upstream_connections",
			Help: "Number of upstream connections.",
		}, []string{"addr"}),
		UpstreamTotalConnections: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "trabbits_upstream_connections_total",
			Help: "Total number of upstream connections.",
		}, []string{"addr"}),
		UpstreamConnectionErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "trabbits_upstream_connection_errors_total",
			Help: "Number of upstream connection errors.",
		}, []string{"addr"}),

		UpstreamHealthyNodes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "trabbits_upstream_healthy_nodes",
			Help: "Number of healthy nodes in upstream cluster.",
		}, []string{"upstream"}),
		UpstreamUnhealthyNodes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "trabbits_upstream_unhealthy_nodes",
			Help: "Number of unhealthy nodes in upstream cluster.",
		}, []string{"upstream"}),

		ProcessedMessages: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "trabbits_processed_messages_total",
			Help: "Number of processed messages by method.",
		}, []string{"method"}),
		ErroredMessages: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "trabbits_errored_messages_total",
			Help: "Number of errored messages by method.",
		}, []string{"method"}),

		LoggerStats: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "trabbits_logger_stats_total",
			Help: "Number of logger stats by level.",
		}, []string{"level"}),

		PanicRecoveries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "trabbits_panic_recoveries_total",
			Help: "Number of panic recoveries by function.",
		}, []string{"function"}),
	}
}

func (m *Metrics) MustRegister(reg prometheus.Registerer) {
	reg.MustRegister(
		m.ClientConnections,
		m.ClientTotalConnections,
		m.ClientConnectionErrors,

		m.ClientReceivedFrames,
		m.ClientSentFrames,

		m.UpstreamConnections,
		m.UpstreamTotalConnections,
		m.UpstreamConnectionErrors,

		m.UpstreamHealthyNodes,
		m.UpstreamUnhealthyNodes,

		m.ProcessedMessages,
		m.ErroredMessages,

		m.LoggerStats,

		m.PanicRecoveries,
	)
}

func runMetricsServer(ctx context.Context, opt *CLI) (func(), error) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(metricsReg, promhttp.HandlerOpts{}))
	var srv http.Server
	// start metrics server
	ch := make(chan error)
	go func() {
		slog.Info("starting metrics server", "port", opt.MetricsPort)
		srv := &http.Server{
			Handler: mux,
			Addr:    fmt.Sprintf(":%d", opt.MetricsPort),
		}
		if err := srv.ListenAndServe(); err != nil {
			slog.Error("failed to start metrics server", "error", err)
			ch <- err
		}
	}()
	wait := time.NewTimer(100 * time.Millisecond)
	select {
	case err := <-ch:
		return nil, err
	case <-wait.C:
		slog.Info("metrics server started", "port", opt.MetricsPort)
	}
	return func() { srv.Shutdown(ctx) }, nil
}

// SetHealthyNodes sets the number of healthy nodes for a given upstream
func (m *Metrics) SetHealthyNodes(upstream string, count float64) {
	m.UpstreamHealthyNodes.WithLabelValues(upstream).Set(count)
}

// SetUnhealthyNodes sets the number of unhealthy nodes for a given upstream
func (m *Metrics) SetUnhealthyNodes(upstream string, count float64) {
	m.UpstreamUnhealthyNodes.WithLabelValues(upstream).Set(count)
}
