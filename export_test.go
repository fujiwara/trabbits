package trabbits

import "time"

var (
	Boot               = boot
	SetupLogger        = setupLogger
	NewDelivery        = newDelivery
	RestoreDeliveryTag = restoreDeliveryTag
	MatchPattern       = matchPattern
	StoreConfig        = storeConfig
	MustGetConfig      = mustGetConfig
	MetricsStore       = metrics
	RunAPIServer       = runAPIServer
	NewAPIClient       = newAPIClient
)

type Delivery = delivery

func init() {
	FrameMax = 256 // for testing
}

func SetReadTimeout(t time.Duration) {
	readTimeout = t
}

func GetReadTimeout() time.Duration {
	return readTimeout
}
