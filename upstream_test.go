package trabbits_test

import (
	"log/slog"
	"testing"

	"github.com/fujiwara/trabbits"
	"github.com/fujiwara/trabbits/amqp091"
	"github.com/fujiwara/trabbits/config"
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
	attr *config.QueueAttributes

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
		attr:       &config.QueueAttributes{},
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
		attr: &config.QueueAttributes{
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
	{
		name: "partial override - keep client auto_delete",
		m: &amqp091.QueueDeclare{
			Queue:      "bar",
			Durable:    false,
			AutoDelete: true, // client wants auto_delete
			Exclusive:  false,
			Arguments:  nil,
		},
		attr: &config.QueueAttributes{
			Durable: ptr(true), // override durable only
			// AutoDelete is nil, so client's value should be kept
			Arguments: amqp091.Table{
				"x-queue-type": "quorum",
			},
		},
		queue:      "bar",
		durable:    true,  // overridden
		autoDelete: true,  // kept from client
		exclusive:  false, // kept from client
		args: rabbitmq.Table{
			"x-queue-type": "quorum",
		},
	},
	{
		name: "partial override - keep client exclusive",
		m: &amqp091.QueueDeclare{
			Queue:      "baz",
			Durable:    false,
			AutoDelete: false,
			Exclusive:  true, // client wants exclusive
			Arguments:  nil,
		},
		attr: &config.QueueAttributes{
			Durable:    ptr(true),
			AutoDelete: ptr(false),
			// Exclusive is nil, so client's value should be kept
		},
		queue:      "baz",
		durable:    true,  // overridden
		autoDelete: false, // overridden
		exclusive:  true,  // kept from client
		args:       nil,
	},
	{
		name: "try_passive enabled - normal declare args",
		m: &amqp091.QueueDeclare{
			Queue:      "test",
			Durable:    false,
			AutoDelete: true,
			Exclusive:  false,
			Arguments:  nil,
		},
		attr: &config.QueueAttributes{
			Durable:    ptr(true),
			TryPassive: true, // enable try_passive
			Arguments: amqp091.Table{
				"x-queue-type": "quorum",
			},
		},
		queue:      "test",
		durable:    true, // overridden for normal declare
		autoDelete: true, // kept from client
		exclusive:  false,
		args: rabbitmq.Table{
			"x-queue-type": "quorum",
		},
	},
}

func TestUpstreamQueueAttr(t *testing.T) {
	for _, tc := range testUpstreamQueueAttrSuites {
		t.Run(tc.name, func(t *testing.T) {
			// Create a dummy metrics instance for testing
			cfg := &config.Config{}
			server := trabbits.NewServer(cfg, "/tmp/test-upstream.sock")
			u := trabbits.NewUpstream(nil, slog.Default(), config.Upstream{QueueAttributes: tc.attr}, "test:5672", server.Metrics())
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
