package trabbits_test

import (
	"testing"

	"github.com/fujiwara/trabbits"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricsLinting(t *testing.T) {
	reg := trabbits.GetMetricsRegistry()

	problems, err := testutil.CollectAndLint(reg)
	if err != nil {
		t.Errorf("Failed to collect and lint metrics: %v", err)
	}
	for _, p := range problems {
		t.Errorf("Metric linting problem: %s - %s", p.Metric, p.Text)
	}
}
