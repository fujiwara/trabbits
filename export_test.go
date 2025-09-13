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
	return NewServer(config, "") // Empty API socket for tests
}

func (s *Server) TestDisconnectOutdatedProxies(currentConfigHash string) <-chan int {
	return s.disconnectOutdatedProxies(currentConfigHash)
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
