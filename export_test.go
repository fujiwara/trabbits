package trabbits

import (
	"context"
	"net"

	"github.com/fujiwara/trabbits/config"
	"github.com/fujiwara/trabbits/pattern"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	SetupLogger        = setupLogger
	NewDelivery        = newDelivery
	RestoreDeliveryTag = restoreDeliveryTag
	MatchPattern       = pattern.Match
	MetricsStore       = metrics
	NewAPIClient       = newAPIClient
	TestMatchRouting   = testMatchRouting
)

// Server instance functions for testing
func NewTestServer(config *config.Config) *Server {
	config.SetDefaults()         // Set default values for testing
	return NewServer(config, "") // Empty API socket for tests
}

func (s *Server) TestDisconnectOutdatedProxies(currentConfigHash string) <-chan int {
	return s.disconnectOutdatedProxies(currentConfigHash)
}

func (s *Server) TestDisconnectAllProxies() <-chan int {
	return s.disconnectAllProxies()
}

func (s *Server) TestBoot(ctx context.Context, listener net.Listener) error {
	return s.boot(ctx, listener)
}

func (s *Server) TestStartAPIServer(ctx context.Context, configPath string) (func(), error) {
	return s.startAPIServer(ctx, configPath)
}

type Delivery = delivery

func init() {
	FrameMax = 256 // for testing
}

func GetMetricsRegistry() *prometheus.Registry {
	return metricsReg
}

// Test-only method for setting proxy config hash
func (p *Proxy) SetConfigHash(hash string) {
	p.configHash = hash
}

// Test-only method for setting proxy shutdown message
func (p *Proxy) SetShutdownMessage(message string) {
	p.shutdownMessage = message
}

// Test-only method for gracefully stopping a specific proxy by client address
func (s *Server) TestGracefulStopProxy(clientAddr string, shutdownMessage string) bool {
	var found bool
	s.activeProxies.Range(func(key, value interface{}) bool {
		entry := value.(*proxyEntry)
		if entry.proxy.ClientAddr() == clientAddr {
			// Set custom shutdown message
			entry.proxy.shutdownMessage = shutdownMessage
			// Trigger graceful shutdown
			entry.cancel()
			found = true
			return false // Stop iteration
		}
		return true
	})
	return found
}
