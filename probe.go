package trabbits

import (
	"sync"
	"time"
)

type probeLog struct {
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
	attrs     []any     // Store as slice to avoid allocation when not consumed
}

// AttrsMap converts the internal attrs slice to a map when needed (e.g., for JSON marshaling)
func (p *probeLog) AttrsMap() map[string]any {
	if len(p.attrs) == 0 {
		return nil
	}

	m := make(map[string]any)
	for i := 0; i < len(p.attrs)-1; i += 2 {
		if key, ok := p.attrs[i].(string); ok {
			m[key] = p.attrs[i+1]
		}
	}
	return m
}

// ProbeLogBuffer stores probe logs for a proxy with a circular buffer
type ProbeLogBuffer struct {
	mu             sync.RWMutex
	logs           []probeLog
	maxSize        int
	active         bool      // whether the proxy is still active
	disconnectedAt time.Time // timestamp when the proxy disconnected

	// Proxy information (captured at registration for disconnected proxies)
	clientAddr   string
	user         string
	virtualHost  string
	clientBanner string
	connectedAt  time.Time
}

// NewProbeLogBuffer creates a new probe log buffer
func NewProbeLogBuffer(maxSize int) *ProbeLogBuffer {
	return &ProbeLogBuffer{
		logs:    make([]probeLog, 0, maxSize),
		maxSize: maxSize,
		active:  true,
	}
}

// Add adds a probe log to the buffer
func (b *ProbeLogBuffer) Add(log probeLog) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.logs = append(b.logs, log)
	if len(b.logs) > b.maxSize {
		// Keep only the last maxSize entries
		b.logs = b.logs[len(b.logs)-b.maxSize:]
	}
}

// GetLogs returns a copy of all logs
func (b *ProbeLogBuffer) GetLogs() []probeLog {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]probeLog, len(b.logs))
	copy(result, b.logs)
	return result
}

// MarkInactive marks the buffer as inactive (proxy disconnected)
func (b *ProbeLogBuffer) MarkInactive() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.active = false
	b.disconnectedAt = time.Now()
}

// GetDisconnectedAt returns the timestamp when the proxy disconnected
func (b *ProbeLogBuffer) GetDisconnectedAt() time.Time {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.disconnectedAt
}

// IsActive returns whether the proxy is still active
func (b *ProbeLogBuffer) IsActive() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.active
}

// SetProxyInfo sets the proxy information (should be called once at registration)
func (b *ProbeLogBuffer) SetProxyInfo(clientAddr, user, virtualHost, clientBanner string, connectedAt time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clientAddr = clientAddr
	b.user = user
	b.virtualHost = virtualHost
	b.clientBanner = clientBanner
	b.connectedAt = connectedAt
}

// GetProxyInfo returns the stored proxy information
func (b *ProbeLogBuffer) GetProxyInfo() (clientAddr, user, virtualHost, clientBanner string, connectedAt time.Time) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.clientAddr, b.user, b.virtualHost, b.clientBanner, b.connectedAt
}
