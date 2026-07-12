package metrics

import (
	"sync"
	"sync/atomic"
)

// Counter is a monotonically increasing counter.
// Its state is held in memory and observed by OpenTelemetry observable
// instruments at collection time (see NewMetrics).
type Counter struct {
	v atomic.Int64
}

// Inc increments the counter by 1.
func (c *Counter) Inc() { c.v.Add(1) }

// Add adds the given value to the counter.
func (c *Counter) Add(d int64) { c.v.Add(d) }

// Value returns the current value of the counter.
func (c *Counter) Value() int64 { return c.v.Load() }

// Gauge is a value that can go up and down.
type Gauge struct {
	v atomic.Int64
}

// Inc increments the gauge by 1.
func (g *Gauge) Inc() { g.v.Add(1) }

// Dec decrements the gauge by 1.
func (g *Gauge) Dec() { g.v.Add(-1) }

// Set sets the gauge to the given value.
func (g *Gauge) Set(v int64) { g.v.Store(v) }

// Value returns the current value of the gauge.
func (g *Gauge) Value() int64 { return g.v.Load() }

// CounterVec is a set of Counters partitioned by a single label value.
type CounterVec struct {
	mu   sync.RWMutex
	vals map[string]*Counter
}

func newCounterVec() *CounterVec {
	return &CounterVec{vals: make(map[string]*Counter)}
}

// WithLabelValues returns the Counter for the given label value,
// creating it if it does not exist yet.
func (v *CounterVec) WithLabelValues(lv string) *Counter {
	v.mu.RLock()
	c, ok := v.vals[lv]
	v.mu.RUnlock()
	if ok {
		return c
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if c, ok := v.vals[lv]; ok {
		return c
	}
	c = &Counter{}
	v.vals[lv] = c
	return c
}

// Reset removes all label values from the vector.
func (v *CounterVec) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()
	clear(v.vals)
}

func (v *CounterVec) each(f func(labelValue string, value int64)) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	for lv, c := range v.vals {
		f(lv, c.Value())
	}
}

// GaugeVec is a set of Gauges partitioned by a single label value.
type GaugeVec struct {
	mu   sync.RWMutex
	vals map[string]*Gauge
}

func newGaugeVec() *GaugeVec {
	return &GaugeVec{vals: make(map[string]*Gauge)}
}

// WithLabelValues returns the Gauge for the given label value,
// creating it if it does not exist yet.
func (v *GaugeVec) WithLabelValues(lv string) *Gauge {
	v.mu.RLock()
	g, ok := v.vals[lv]
	v.mu.RUnlock()
	if ok {
		return g
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if g, ok := v.vals[lv]; ok {
		return g
	}
	g = &Gauge{}
	v.vals[lv] = g
	return g
}

// Reset removes all label values from the vector.
func (v *GaugeVec) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()
	clear(v.vals)
}

func (v *GaugeVec) each(f func(labelValue string, value int64)) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	for lv, g := range v.vals {
		f(lv, g.Value())
	}
}
