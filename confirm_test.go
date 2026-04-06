package trabbits_test

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/fujiwara/trabbits"
	rabbitmq "github.com/rabbitmq/amqp091-go"
)

func TestConfirmStateTagMapping(t *testing.T) {
	t.Parallel()
	cs := trabbits.NewConfirmState()

	// Simulate publishes to different upstreams
	// Publish 1 → upstream 0, upstream tag 1
	clientTag1 := cs.RecordPublish(0, 1)
	if clientTag1 != 1 {
		t.Errorf("expected client tag 1, got %d", clientTag1)
	}

	// Publish 2 → upstream 1, upstream tag 1
	clientTag2 := cs.RecordPublish(1, 1)
	if clientTag2 != 2 {
		t.Errorf("expected client tag 2, got %d", clientTag2)
	}

	// Publish 3 → upstream 0, upstream tag 2
	clientTag3 := cs.RecordPublish(0, 2)
	if clientTag3 != 3 {
		t.Errorf("expected client tag 3, got %d", clientTag3)
	}

	// Lookup confirms
	tag, ok := cs.LookupClientTag(1, 1)
	if !ok || tag != 2 {
		t.Errorf("expected client tag 2 for upstream 1 tag 1, got %d (ok=%v)", tag, ok)
	}

	tag, ok = cs.LookupClientTag(0, 1)
	if !ok || tag != 1 {
		t.Errorf("expected client tag 1 for upstream 0 tag 1, got %d (ok=%v)", tag, ok)
	}

	tag, ok = cs.LookupClientTag(0, 2)
	if !ok || tag != 3 {
		t.Errorf("expected client tag 3 for upstream 0 tag 2, got %d (ok=%v)", tag, ok)
	}

	// Already consumed, should not be found
	_, ok = cs.LookupClientTag(0, 1)
	if ok {
		t.Error("expected lookup to fail for already consumed tag")
	}
}

func TestConfirmStateUnknownTag(t *testing.T) {
	t.Parallel()
	cs := trabbits.NewConfirmState()

	_, ok := cs.LookupClientTag(0, 1)
	if ok {
		t.Error("expected lookup to fail for unknown tag")
	}
}

func TestProxyPublishConfirm(t *testing.T) {
	conn := mustTestConn(t)
	defer conn.Close()
	ch := mustTestChannel(t, conn)
	defer ch.Close()

	qName := rand.Text()
	q, err := ch.QueueDeclare(qName, false, true, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Enable confirm mode
	if err := ch.Confirm(false); err != nil {
		t.Fatalf("failed to enable confirm mode: %v", err)
	}

	// Publish multiple messages with deferred confirms
	const msgCount = 5
	confirmations := make([]*rabbitmq.DeferredConfirmation, msgCount)
	for i := range msgCount {
		dc, err := ch.PublishWithDeferredConfirm(
			"",
			q.Name,
			false,
			false,
			rabbitmq.Publishing{
				ContentType: "text/plain",
				Body:        []byte("confirm test message"),
			},
		)
		if err != nil {
			t.Fatalf("failed to publish message %d: %v", i, err)
		}
		confirmations[i] = dc
	}

	// Wait for all confirms
	for i, dc := range confirmations {
		select {
		case <-dc.Done():
			if !dc.Acked() {
				t.Errorf("message %d was nacked", i)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout waiting for confirm of message %d", i)
		}
	}

	// Verify messages were delivered
	for i := range msgCount {
		m, ok, err := ch.Get(q.Name, true)
		if err != nil {
			t.Fatalf("failed to get message %d: %v", i, err)
		}
		if !ok {
			t.Fatalf("message %d not found", i)
		}
		if string(m.Body) != "confirm test message" {
			t.Errorf("unexpected message body: %s", string(m.Body))
		}
	}
}

func TestProxyPublishConfirmWithNotifyPublish(t *testing.T) {
	conn := mustTestConn(t)
	defer conn.Close()
	ch := mustTestChannel(t, conn)
	defer ch.Close()

	qName := rand.Text()
	q, err := ch.QueueDeclare(qName, false, true, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := ch.Confirm(false); err != nil {
		t.Fatalf("failed to enable confirm mode: %v", err)
	}

	// Use NotifyPublish channel
	confirms := ch.NotifyPublish(make(chan rabbitmq.Confirmation, 10))

	const msgCount = 3
	for i := range msgCount {
		if err := ch.Publish("", q.Name, false, false, rabbitmq.Publishing{
			ContentType: "text/plain",
			Body:        []byte("notify test"),
		}); err != nil {
			t.Fatalf("failed to publish message %d: %v", i, err)
		}
	}

	// Collect confirms
	received := 0
	timeout := time.After(5 * time.Second)
	for received < msgCount {
		select {
		case conf := <-confirms:
			if !conf.Ack {
				t.Errorf("message %d was nacked", conf.DeliveryTag)
			}
			received++
		case <-timeout:
			t.Fatalf("timeout waiting for confirms, received %d/%d", received, msgCount)
		}
	}
}
