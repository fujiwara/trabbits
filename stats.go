package trabbits

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/fujiwara/trabbits/types"
)

// ProxyStats holds statistics for a single proxy connection
type ProxyStats struct {
	startedAt      time.Time
	methodCounts   sync.Map // map[string]*int64 - key: method name, value: counter
	receivedFrames int64    // frames received from client (atomic)
	sentFrames     int64    // frames sent to client (atomic)
}

// NewProxyStats creates a new ProxyStats instance
func NewProxyStats() *ProxyStats {
	return &ProxyStats{
		startedAt: time.Now(),
	}
}

// IncrementMethod increments the counter for a specific AMQP method
func (s *ProxyStats) IncrementMethod(method string) {
	// Update local counter only - Prometheus metrics are updated directly in proxy.go
	v, _ := s.methodCounts.LoadOrStore(method, new(int64))
	atomic.AddInt64(v.(*int64), 1)
}

// IncrementReceivedFrames increments the received frame counter
func (s *ProxyStats) IncrementReceivedFrames() {
	// Update local counter only - Prometheus metrics are updated directly in proxy.go
	atomic.AddInt64(&s.receivedFrames, 1)
}

// IncrementSentFrames increments the sent frame counter
func (s *ProxyStats) IncrementSentFrames() {
	// Update local counter only - Prometheus metrics are updated directly in proxy.go
	atomic.AddInt64(&s.sentFrames, 1)
}

// GetMethodCount returns the count for a specific method
func (s *ProxyStats) GetMethodCount(method string) int64 {
	v, ok := s.methodCounts.Load(method)
	if !ok {
		return 0
	}
	return atomic.LoadInt64(v.(*int64))
}

// GetAllMethodCounts returns a map of all method counts
func (s *ProxyStats) GetAllMethodCounts() map[string]int64 {
	result := make(map[string]int64)
	s.methodCounts.Range(func(key, value interface{}) bool {
		method := key.(string)
		count := atomic.LoadInt64(value.(*int64))
		result[method] = count
		return true
	})
	return result
}

// GetReceivedFrames returns the number of frames received from client
func (s *ProxyStats) GetReceivedFrames() int64 {
	return atomic.LoadInt64(&s.receivedFrames)
}

// GetSentFrames returns the number of frames sent to client
func (s *ProxyStats) GetSentFrames() int64 {
	return atomic.LoadInt64(&s.sentFrames)
}

// GetTotalFrames returns the total number of frames (received + sent)
func (s *ProxyStats) GetTotalFrames() int64 {
	return s.GetReceivedFrames() + s.GetSentFrames()
}

// GetTotalMethods returns the total number of methods processed
func (s *ProxyStats) GetTotalMethods() int64 {
	var total int64
	s.methodCounts.Range(func(_, value interface{}) bool {
		total += atomic.LoadInt64(value.(*int64))
		return true
	})
	return total
}

// GetStartedAt returns when the stats collection started
func (s *ProxyStats) GetStartedAt() time.Time {
	return s.startedAt
}

// Snapshot returns a snapshot of current statistics
func (s *ProxyStats) Snapshot() types.StatsSnapshot {
	return types.StatsSnapshot{
		StartedAt:      s.startedAt,
		Methods:        s.GetAllMethodCounts(),
		TotalMethods:   s.GetTotalMethods(),
		ReceivedFrames: s.GetReceivedFrames(),
		SentFrames:     s.GetSentFrames(),
		TotalFrames:    s.GetTotalFrames(),
		Duration:       time.Since(s.startedAt).String(),
	}
}
