package trabbits

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	Boot                      = boot
	SetupLogger               = setupLogger
	NewDelivery               = newDelivery
	RestoreDeliveryTag        = restoreDeliveryTag
	MatchPattern              = matchPattern
	StoreConfig               = storeConfig
	MustGetConfig             = mustGetConfig
	MetricsStore              = metrics
	RunAPIServer              = runAPIServer
	NewAPIClient              = newAPIClient
	ReloadConfigFromFile      = reloadConfigFromFile
	TestMatchRouting          = testMatchRouting
	RegisterProxy             = registerProxy
	UnregisterProxy           = unregisterProxy
	GetProxy                  = getProxy
	CountActiveProxies        = countActiveProxies
	ClearActiveProxies        = clearActiveProxies
	DisconnectOutdatedProxies = disconnectOutdatedProxies
)

type Delivery = delivery

func init() {
	FrameMax = 256 // for testing
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
