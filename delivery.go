package trabbits

import (
	"fmt"

	rabbitmq "github.com/rabbitmq/amqp091-go"
)

type delivery struct {
	ch <-chan rabbitmq.Delivery
	i  int
	n  int
}

func newDelivery(ch <-chan rabbitmq.Delivery, i, n int) *delivery {
	return &delivery{
		ch: ch,
		i:  i,
		n:  n,
	}
}

func (d *delivery) Tag(s uint64) uint64 {
	return s*uint64(d.n) + uint64(d.i)
}

func deliveryTagToIndex(tag uint64, n int) int {
	return int(tag % uint64(n))
}

func (s *Proxy) UpstreamDeliveryTag(tag uint64) uint64 {
	n := len(s.Upstreams())
	return tag / uint64(n)
}

func (s *Proxy) GetChannelByDeliveryTag(channelID uint16, tag uint64) (*rabbitmq.Channel, error) {
	chs, err := s.GetChannels(channelID)
	if err != nil {
		return nil, err
	}
	i := deliveryTagToIndex(tag, len(chs))
	if len(chs) <= i {
		return nil, fmt.Errorf("channel not found: id=%d, tag=%d", channelID, tag)
	}
	return chs[i], nil
}

func (s *Proxy) newDelivery(ch <-chan rabbitmq.Delivery, channelID uint16, index int) *delivery {
	return newDelivery(ch, index, len(s.Upstreams()))
}
