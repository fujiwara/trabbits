package trabbits

var (
	Boot               = boot
	SetupLogger        = setupLogger
	NewDelivery        = newDelivery
	RestoreDeliveryTag = restoreDeliveryTag
	MatchPattern       = matchPattern
)

type Delivery = delivery

func init() {
	FrameMax = 256 // for testing
}
