// MIT License
// Copyright (c) 2025 FUJIWARA Shunichiro

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

func restoreDeliveryTag(tag uint64, n int) (t uint64, index int) {
	return tag / uint64(n), int(tag % uint64(n))
}

func (s *Proxy) UpstreamDeliveryTag(tag uint64) uint64 {
	t, _ := restoreDeliveryTag(tag, len(s.Upstreams()))
	return t
}

func (s *Proxy) GetChannelByDeliveryTag(channelID uint16, tag uint64) (*rabbitmq.Channel, error) {
	chs, err := s.GetChannels(channelID)
	if err != nil {
		return nil, err
	}
	_, index := restoreDeliveryTag(tag, len(s.Upstreams()))
	if len(chs) <= index {
		return nil, fmt.Errorf("channel not found: id=%d, tag=%d", channelID, tag)
	}
	return chs[index], nil
}

func (s *Proxy) newDelivery(ch <-chan rabbitmq.Delivery, index int) *delivery {
	return newDelivery(ch, index, len(s.Upstreams()))
}
