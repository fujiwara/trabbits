package trabbits_test

import (
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"github.com/fujiwara/trabbits"
	"github.com/rabbitmq/amqp091-go"
)

func TestProxyPublishGetRouting(t *testing.T) {
	conn := mustTestConn(t)
	defer conn.Close()
	ch := mustTestChannel(t, conn)
	defer ch.Close()

	body := strings.Repeat(rand.Text(), 10)
	if len(body) < trabbits.FrameMax {
		t.Fatal("message is too short")
	}
	testID := rand.Text()
	for _, upstream := range []string{"default", "another"} {
		qName := "test.queue." + upstream + "." + testID
		q, err := ch.QueueDeclare(
			qName, // name
			false, // durable
			true,  // delete when unused
			false, // exclusive
			false, // no-wait
			nil,   // arguments
		)
		if err != nil {
			t.Fatal(err)
		}

		if err := ch.Publish(
			"",     // exchange
			q.Name, // routing key
			false,  // mandatory
			false,  // immediate
			amqp091.Publishing{
				ContentType: "text/plain",
				Body:        []byte(body),
			},
		); err != nil {
			t.Fatal(err)
		} else {
			logger.Info("message published", "upstream", upstream, "queue", q.Name)
		}
	}

	time.Sleep(10 * time.Millisecond) // Wait for the message to be delivered

	for _, upstream := range []string{"default", "another"} {
		qName := "test.queue." + upstream + "." + testID
		m, ok, err := ch.Get(qName, false)
		if err != nil {
			t.Error(err)
		}
		if !ok {
			t.Errorf("message not found")
		}
		logger.Info("message received", "message", m)
		if string(m.Body) != body {
			t.Errorf("unexpected message: %s", string(m.Body))
		}
		if err := ch.Ack(m.DeliveryTag, false); err != nil {
			t.Error(err)
		}
		ch.QueueDelete(qName, false, false, false)
	}
}
