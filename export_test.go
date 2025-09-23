package trabbits

import (
	"context"
	"net"
	"net/http"

	"github.com/fujiwara/trabbits/config"
	"github.com/fujiwara/trabbits/pattern"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	SetupLogger        = setupLogger
	NewDelivery        = newDelivery
	RestoreDeliveryTag = restoreDeliveryTag
	MatchPattern       = pattern.Match
	TestMatchRouting   = testMatchRouting
	RecoverFromPanic   = recoverFromPanic
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
	initMetricsRegistry()
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

// Test-only method for setting proxy user
func (p *Proxy) SetUser(user string) {
	p.user = user
}

// Test-only method for setting proxy virtual host
func (p *Proxy) SetVirtualHost(vhost string) {
	p.VirtualHost = vhost
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

// Test-only method for accessing proxy statistics
func (p *Proxy) Stats() *ProxyStats {
	return p.stats
}

// Test-only method for accessing sortNodesByLeastConnections
func (p *Proxy) SortNodesByLeastConnections(nodes []string) []string {
	return p.sortNodesByLeastConnections(nodes)
}

// Export manage functions for testing
var (
	ManageConfig        = manageConfig
	ManageClients       = manageClients
	ManageProxyInfo     = manageProxyInfo
	ManageProxyShutdown = manageProxyShutdown
)

// TUI functionality moved to tui package

// Export probe log related types and methods for testing
type ProbeLog = probeLog

// SendProbeLog exports the sendProbeLog method for testing
func (p *Proxy) SendProbeLog(message string, attrs ...any) {
	p.sendProbeLog(message, attrs...)
}

// SetProbeChan sets the probe channel for testing
func (p *Proxy) SetProbeChan(ch chan probeLog) {
	p.probeChan = ch
}

// GetAPIProbeLogHandler exports the API probe log handler for testing
func (s *Server) GetAPIProbeLogHandler() http.HandlerFunc {
	return s.apiProbeLogHandler()
}
