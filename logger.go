package trabbits

import (
	"context"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
)

type MetricSlogHandler struct {
	slog.Handler
	logCounter *prometheus.CounterVec
}

func NewMetricSlogHandler(base slog.Handler, logCounter *prometheus.CounterVec) slog.Handler {
	if logCounter != nil {
		logCounter.WithLabelValues("INFO").Add(0)
		logCounter.WithLabelValues("WARN").Add(0)
		logCounter.WithLabelValues("ERROR").Add(0)
	}
	return &MetricSlogHandler{
		Handler:    base,
		logCounter: logCounter,
	}
}

func (h *MetricSlogHandler) Handle(ctx context.Context, r slog.Record) error {
	if h.logCounter != nil {
		h.logCounter.WithLabelValues(r.Level.String()).Inc()
	}
	return h.Handler.Handle(ctx, r)
}
