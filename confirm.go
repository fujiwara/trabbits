// MIT License
// Copyright (c) 2025 FUJIWARA Shunichiro

package trabbits

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/fujiwara/trabbits/amqp091"
	rabbitmq "github.com/rabbitmq/amqp091-go"
)

type confirmState struct {
	mu        sync.Mutex
	clientTag uint64                // current client delivery tag counter (incremented per publish)
	tagMap    map[confirmKey]uint64 // (upstream_index, upstream_tag) -> client_tag
}

type confirmKey struct {
	upstreamIndex int
	upstreamTag   uint64
}

func newConfirmState() *confirmState {
	return &confirmState{
		tagMap: make(map[confirmKey]uint64),
	}
}

// recordPublish records a mapping from upstream tag to client tag.
// Returns the client-side delivery tag.
func (cs *confirmState) RecordPublish(upstreamIndex int, upstreamTag uint64) uint64 {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.clientTag++
	cs.tagMap[confirmKey{upstreamIndex: upstreamIndex, upstreamTag: upstreamTag}] = cs.clientTag
	return cs.clientTag
}

// lookupClientTag returns the client-side delivery tag for a given upstream confirmation
// and removes the mapping entry.
func (cs *confirmState) LookupClientTag(upstreamIndex int, upstreamTag uint64) (uint64, bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	key := confirmKey{upstreamIndex: upstreamIndex, upstreamTag: upstreamTag}
	tag, ok := cs.tagMap[key]
	if ok {
		delete(cs.tagMap, key)
	}
	return tag, ok
}

func (p *Proxy) getConfirmState(channelID uint16) *confirmState {
	if p.confirmStates == nil {
		return nil
	}
	return p.confirmStates[channelID]
}

func (p *Proxy) setConfirmState(channelID uint16, cs *confirmState) {
	if p.confirmStates == nil {
		p.confirmStates = make(map[uint16]*confirmState)
	}
	p.confirmStates[channelID] = cs
}

func (p *Proxy) deleteConfirmState(channelID uint16) {
	if p.confirmStates != nil {
		delete(p.confirmStates, channelID)
	}
}

func (p *Proxy) replyConfirmSelect(_ context.Context, f *amqp091.MethodFrame, m *amqp091.ConfirmSelect) error {
	id := f.Channel()
	p.probeLog("c->t Confirm.Select", "channel", id, "nowait", m.Nowait)

	chs, err := p.GetChannels(id)
	if err != nil {
		return err
	}

	// Put all upstream channels into confirm mode and start listeners
	cs := newConfirmState()
	for i, ch := range chs {
		us := p.Upstream(i)
		us.probeLog("t->u Confirm.Select", "channel", id)
		if err := ch.Confirm(false); err != nil {
			return NewError(amqp091.InternalError, fmt.Sprintf("failed to enable confirm mode on upstream %s: %v", us.String(), err))
		}
		notifyChan := ch.NotifyPublish(make(chan rabbitmq.Confirmation, 64))
		go p.listenConfirm(id, i, notifyChan, cs)
	}

	p.setConfirmState(id, cs)

	if !m.Nowait {
		return p.send(id, &amqp091.ConfirmSelectOk{})
	}
	return nil
}

func (p *Proxy) listenConfirm(channelID uint16, upstreamIndex int, confirmChan <-chan rabbitmq.Confirmation, cs *confirmState) {
	defer recoverFromPanic(p.logger, "listenConfirm", p.metrics)
	us := p.Upstream(upstreamIndex)
	for conf := range confirmChan {
		clientTag, ok := cs.LookupClientTag(upstreamIndex, conf.DeliveryTag)
		if !ok {
			p.logger.Warn("confirm for unknown tag",
				"upstream", us.String(),
				"upstream_tag", conf.DeliveryTag,
			)
			continue
		}
		us.probeLog("t<-u confirm", "upstream_tag", conf.DeliveryTag, "client_tag", clientTag, "ack", conf.Ack)
		var err error
		if conf.Ack {
			err = p.send(channelID, &amqp091.BasicAck{
				DeliveryTag: clientTag,
				Multiple:    false,
			})
		} else {
			err = p.send(channelID, &amqp091.BasicNack{
				DeliveryTag: clientTag,
				Multiple:    false,
				Requeue:     false,
			})
		}
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) || isBrokenPipe(err) {
				return
			}
			p.logger.Warn("failed to send confirm to client", "error", err)
			return
		}
	}
}
