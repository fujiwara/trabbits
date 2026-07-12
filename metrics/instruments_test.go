package metrics_test

import (
	"sync"
	"testing"

	"github.com/fujiwara/trabbits/metrics"
)

func TestCounter(t *testing.T) {
	store := metrics.NewTestStore()
	c := store.Metrics().ClientTotalConnections

	if v := c.Value(); v != 0 {
		t.Errorf("initial value = %d, want 0", v)
	}
	c.Inc()
	c.Inc()
	c.Add(3)
	if v := c.Value(); v != 5 {
		t.Errorf("value = %d, want 5", v)
	}
}

func TestGauge(t *testing.T) {
	store := metrics.NewTestStore()
	g := store.Metrics().ClientConnections

	g.Inc()
	g.Inc()
	g.Dec()
	if v := g.Value(); v != 1 {
		t.Errorf("value = %d, want 1", v)
	}
	g.Set(10)
	if v := g.Value(); v != 10 {
		t.Errorf("value = %d, want 10", v)
	}
}

func TestCounterVec(t *testing.T) {
	store := metrics.NewTestStore()
	vec := store.Metrics().ProcessedMessages

	vec.WithLabelValues("Basic.Publish").Inc()
	vec.WithLabelValues("Basic.Publish").Inc()
	vec.WithLabelValues("Basic.Get").Inc()

	if v := vec.WithLabelValues("Basic.Publish").Value(); v != 2 {
		t.Errorf("Basic.Publish = %d, want 2", v)
	}
	if v := vec.WithLabelValues("Basic.Get").Value(); v != 1 {
		t.Errorf("Basic.Get = %d, want 1", v)
	}
	// WithLabelValues must return the same instance for the same label
	c1 := vec.WithLabelValues("Basic.Publish")
	c2 := vec.WithLabelValues("Basic.Publish")
	if c1 != c2 {
		t.Error("WithLabelValues returned different instances for the same label")
	}

	vec.Reset()
	if v := vec.WithLabelValues("Basic.Publish").Value(); v != 0 {
		t.Errorf("value after Reset = %d, want 0", v)
	}
}

func TestGaugeVec(t *testing.T) {
	store := metrics.NewTestStore()
	vec := store.Metrics().UpstreamConnections

	vec.WithLabelValues("host1:5672").Inc()
	vec.WithLabelValues("host1:5672").Inc()
	vec.WithLabelValues("host2:5672").Set(5)
	vec.WithLabelValues("host2:5672").Dec()

	if v := vec.WithLabelValues("host1:5672").Value(); v != 2 {
		t.Errorf("host1 = %d, want 2", v)
	}
	if v := vec.WithLabelValues("host2:5672").Value(); v != 4 {
		t.Errorf("host2 = %d, want 4", v)
	}

	vec.Reset()
	if v := vec.WithLabelValues("host1:5672").Value(); v != 0 {
		t.Errorf("value after Reset = %d, want 0", v)
	}
}

func TestInstrumentsConcurrency(t *testing.T) {
	store := metrics.NewTestStore()
	m := store.Metrics()

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for range 100 {
				m.ClientTotalConnections.Inc()
				m.ClientConnections.Inc()
				m.ClientConnections.Dec()
				m.ProcessedMessages.WithLabelValues("Basic.Publish").Inc()
				m.UpstreamConnections.WithLabelValues("host1:5672").Inc()
			}
		})
	}
	wg.Wait()

	if v := m.ClientTotalConnections.Value(); v != 1000 {
		t.Errorf("ClientTotalConnections = %d, want 1000", v)
	}
	if v := m.ClientConnections.Value(); v != 0 {
		t.Errorf("ClientConnections = %d, want 0", v)
	}
	if v := m.ProcessedMessages.WithLabelValues("Basic.Publish").Value(); v != 1000 {
		t.Errorf("ProcessedMessages = %d, want 1000", v)
	}
	if v := m.UpstreamConnections.WithLabelValues("host1:5672").Value(); v != 1000 {
		t.Errorf("UpstreamConnections = %d, want 1000", v)
	}
}

func TestSetHealthyNodes(t *testing.T) {
	store := metrics.NewTestStore()
	m := store.Metrics()

	m.SetHealthyNodes("cluster1", 3)
	m.SetUnhealthyNodes("cluster1", 1)

	if v := m.UpstreamHealthyNodes.WithLabelValues("cluster1").Value(); v != 3 {
		t.Errorf("healthy nodes = %d, want 3", v)
	}
	if v := m.UpstreamUnhealthyNodes.WithLabelValues("cluster1").Value(); v != 1 {
		t.Errorf("unhealthy nodes = %d, want 1", v)
	}
}
