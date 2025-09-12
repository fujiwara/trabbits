package trabbits

import (
	"context"
	"net"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	SetupLogger        = setupLogger
	NewDelivery        = newDelivery
	RestoreDeliveryTag = restoreDeliveryTag
	MatchPattern       = matchPattern
	MetricsStore       = metrics
	RunAPIServer       = runAPIServer
	NewAPIClient       = newAPIClient
	TestMatchRouting   = testMatchRouting
	GetProxy           = getProxy
	CountActiveProxies = countActiveProxies
	ClearActiveProxies = clearActiveProxies
)

// Global test server instance - set by server_test.go for use by routing_test.go
var TestServer *Server

// Server instance functions for testing
func NewTestServer(config *Config) *Server {
	return NewServer(config)
}

func (s *Server) TestNewProxy(conn net.Conn) *Proxy {
	return s.NewProxy(conn)
}

func (s *Server) TestRegisterProxy(proxy *Proxy) {
	s.registerProxy(proxy)
}

func (s *Server) TestUnregisterProxy(proxy *Proxy) {
	s.unregisterProxy(proxy)
}

func (s *Server) TestDisconnectOutdatedProxies(currentConfigHash string) <-chan int {
	return s.disconnectOutdatedProxies(currentConfigHash)
}

func (s *Server) TestBoot(ctx context.Context, listener net.Listener) error {
	return s.boot(ctx, listener)
}

// Test helper for reloadConfigFromFile
func ReloadConfigFromFile(ctx context.Context, configPath string) (*Config, error) {
	// Create a temporary server instance for config reloading
	cfg, err := LoadConfig(ctx, configPath)
	if err != nil {
		return nil, err
	}
	server := NewServer(cfg)
	return reloadConfigFromFile(ctx, configPath, server)
}

type Delivery = delivery

func init() {
	FrameMax = 256                                 // for testing
	connectionCloseTimeout = 50 * time.Millisecond // shorter timeout for tests
}

func SetReadTimeout(t time.Duration) {
	readTimeout = t
}

func GetReadTimeout() time.Duration {
	return readTimeout
}

func SetConnectionCloseTimeout(t time.Duration) {
	connectionCloseTimeout = t
}

func GetConnectionCloseTimeout() time.Duration {
	return connectionCloseTimeout
}

func GetMetricsRegistry() *prometheus.Registry {
	return metricsReg
}
