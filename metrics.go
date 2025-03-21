package trabbits

import "github.com/prometheus/client_golang/prometheus"

var metrics *Metrics

func init() {
	metrics = NewMetrics()
	metrics.MustRegister()
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

	ProcessedMessages *prometheus.CounterVec
	ErroredMessages   *prometheus.CounterVec
}

func NewMetrics() *Metrics {
	return &Metrics{
		ClientConnections: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "trabbits_client_connections",
			Help: "Number of client connections.",
		}),
		ClientTotalConnections: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "trabbits_client_total_connections",
			Help: "Total number of client connections.",
		}),
		ClientConnectionErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "trabbits_client_connection_errors",
			Help: "Number of client connection errors.",
		}),

		ClientReceivedFrames: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "trabbits_client_received_frames",
			Help: "Number of received frames from clients.",
		}),
		ClientSentFrames: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "trabbits_client_sent_frames",
			Help: "Number of sent frames to clients.",
		}),

		UpstreamConnections: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "trabbits_upstream_connections",
			Help: "Number of upstream connections.",
		}, []string{"addr"}),
		UpstreamTotalConnections: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "trabbits_upstream_total_connections",
			Help: "Total number of upstream connections.",
		}, []string{"addr"}),
		UpstreamConnectionErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "trabbits_upstream_connection_errors",
			Help: "Number of upstream connection errors.",
		}, []string{"addr"}),

		ProcessedMessages: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "trabbits_processed_messages",
			Help: "Number of processed messages by method.",
		}, []string{"method"}),
		ErroredMessages: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "trabbits_errored_messages",
			Help: "Number of errored messages by method.",
		}, []string{"method"}),
	}
}

func (m *Metrics) MustRegister() {
	prometheus.MustRegister(
		m.ClientConnections,
		m.ClientTotalConnections,
		m.ClientConnectionErrors,

		m.ClientReceivedFrames,
		m.ClientSentFrames,

		m.UpstreamConnections,
		m.UpstreamTotalConnections,
		m.UpstreamConnectionErrors,

		m.ProcessedMessages,
		m.ErroredMessages,
	)
}
