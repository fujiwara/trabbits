package trabbits_test

import (
	"log/slog"
	"testing"

	"github.com/fujiwara/trabbits"
	"github.com/fujiwara/trabbits/amqp091"
	"github.com/google/go-cmp/cmp"
	rabbitmq "github.com/rabbitmq/amqp091-go"
)

func ptr[T any](v T) *T {
	return &v
}

var testUpstreamQueueAttrSuites = []struct {
	name string
	// input
	m    *amqp091.QueueDeclare
	attr *trabbits.QueueAttributes

	// expected
	queue      string
	durable    bool
	autoDelete bool
	exclusive  bool
	args       rabbitmq.Table
}{
	{
		name:       "nil attr",
		m:          &amqp091.QueueDeclare{Queue: "foo", Arguments: nil},
		attr:       nil,
		queue:      "foo",
		durable:    false,
		autoDelete: false,
		exclusive:  false,
		args:       nil,
	},
	{
		name:       "empty attr",
		m:          &amqp091.QueueDeclare{Queue: "foo", Arguments: nil},
		attr:       &trabbits.QueueAttributes{},
		queue:      "foo",
		durable:    false,
		autoDelete: false,
		exclusive:  false,
		args:       nil,
	},
	{
		name: "auto naming",
		m: &amqp091.QueueDeclare{
			Queue:      trabbits.AutoGenerateQueueNamePrefix + "foo",
			Durable:    true,  // to be overridden
			AutoDelete: false, // to be overridden
			Exclusive:  false, // to be overridden
			Arguments:  nil,
		},
		attr:       nil,
		queue:      trabbits.AutoGenerateQueueNamePrefix + "foo",
		durable:    false,
		autoDelete: true,
		exclusive:  true,
		args:       nil,
	},
	{
		name: "override",
		m: &amqp091.QueueDeclare{
			Queue:      "foo",
			AutoDelete: true,
			Arguments: amqp091.Table{
				"x-queue-type":  "classic", // to be overridden
				"x-message-ttl": 1000,      // to be deleted
				"x-keep":        "me",      // to be kept
			},
		},
		attr: &trabbits.QueueAttributes{
			Durable:    ptr(true),
			AutoDelete: ptr(false),
			Exclusive:  ptr(true),
			Arguments: amqp091.Table{
				"x-queue-type":     "quorum",      // override
				"x-message-ttl":    nil,           // delete
				"max-length-bytes": float64(1024), // add
			},
		},
		queue:      "foo",
		durable:    true,
		autoDelete: false,
		exclusive:  true,
		args: rabbitmq.Table{
			"x-queue-type":     "quorum",
			"max-length-bytes": float64(1024),
			"x-keep":           "me",
		},
	},
}

func TestUpstreamQueueAttr(t *testing.T) {
	for _, tc := range testUpstreamQueueAttrSuites {
		t.Run(tc.name, func(t *testing.T) {
			u := trabbits.NewUpstream(nil, slog.Default(), trabbits.UpstreamConfig{QueueAttributes: tc.attr}, "test:5672")
			queue, durable, autoDelete, exclusive, noWait, args := u.QueueDeclareArgs(tc.m)
			if queue != tc.queue {
				t.Errorf("queue name mismatch: %s != %s", queue, tc.queue)
			}
			if durable != tc.durable {
				t.Errorf("durable mismatch: %t != %t", durable, tc.durable)
			}
			if autoDelete != tc.autoDelete {
				t.Errorf("autoDelete mismatch: %t != %t", autoDelete, tc.autoDelete)
			}
			if exclusive != tc.exclusive {
				t.Errorf("exclusive mismatch: %t != %t", exclusive, tc.exclusive)
			}
			if noWait {
				t.Errorf("noWait should be false")
			}
			if diff := cmp.Diff(args, tc.args); diff != "" {
				t.Errorf("args mismatch: %s", diff)
			}
		})
	}
}
