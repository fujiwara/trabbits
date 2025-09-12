package trabbits

import (
	"context"
	"net"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	SetupLogger          = setupLogger
	NewDelivery          = newDelivery
	RestoreDeliveryTag   = restoreDeliveryTag
	MatchPattern         = matchPattern
	StoreConfig          = storeConfig
	MustGetConfig        = mustGetConfig
	MetricsStore         = metrics
	RunAPIServer         = runAPIServer
	NewAPIClient         = newAPIClient
	ReloadConfigFromFile = reloadConfigFromFile
	TestMatchRouting     = testMatchRouting
	GetProxy             = getProxy
	CountActiveProxies   = countActiveProxies
	ClearActiveProxies   = clearActiveProxies
)

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
