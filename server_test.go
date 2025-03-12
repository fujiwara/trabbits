package trabbits_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fujiwara/trabbits"
	"github.com/rabbitmq/amqp091-go"
)

var testProxyPort int

func runTestProxy(ctx context.Context) error {
	listener, err := net.Listen("tcp", "localhost:0") // Listen on a ephemeral port
	if err != nil {
		slog.Error("Failed to start test server", "error", err)
		return err
	}
	testProxyPort = listener.Addr().(*net.TCPAddr).Port
	if os.Getenv("TEST_PROXY_PORT") != "" {
		testProxyPort, _ = strconv.Atoi(os.Getenv("TEST_PROXY_PORT"))
	}
	go trabbits.Boot(ctx, listener)
	time.Sleep(100 * time.Millisecond) // Wait for the server to start
	return nil
}

var (
	serverCtx, cancel = context.WithCancel(context.Background())
	logger            *slog.Logger
)

func TestMain(m *testing.M) {
	debug, _ := strconv.ParseBool(os.Getenv("DEBUG"))
	trabbits.SetupLogger(debug)
	handler := slog.Default().Handler()
	logger = slog.New(handler).With("test", true)

	runTestProxy(serverCtx)
	defer cancel()
	m.Run()
}

func TestProxyConnect(t *testing.T) {
	conn := mustTestConn(t)
	logger.Info("connected", "conn", conn)
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("channel opened", "ch", ch)
}

func TestProxyChannel(t *testing.T) {
	conn := mustTestConn(t)
	defer conn.Close()
	logger.Info("connected", "conn", conn)

	for i := 0; i < 3; i++ {
		ch, err := conn.Channel()
		if err != nil {
			t.Fatal(err)
		}
		logger.Info("channel opened", "ch", ch)
		if err := ch.Close(); err != nil {
			t.Error("failed to close channel", "error", err)
		}
		if !ch.IsClosed() {
			t.Error("channel is not closed")
		}
	}
}

func TestProxyPublishGet(t *testing.T) {
	conn := mustTestConn(t)
	logger.Info("connected", "conn", conn)
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("channel opened", "ch", ch)

	qName := rand.Text()
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

	body := strings.Repeat(rand.Text(), 10)
	if len(body) < trabbits.FrameMax {
		t.Fatal("message is too short")
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
		logger.Info("message published")
	}

	time.Sleep(10 * time.Millisecond) // Wait for the message to be delivered

	m, ok, err := ch.Get(q.Name, true)
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

	defer ch.Close()
	defer conn.Close()
}

func mustTestConn(t *testing.T) *amqp091.Connection {
	conn, err := amqp091.Dial(fmt.Sprintf("amqp://admin:admin@127.0.0.1:%d/", testProxyPort))
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func TestProxyAckNack(t *testing.T) {
	conn := mustTestConn(t)
	defer conn.Close()
	logger.Info("connected", "conn", conn)
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()
	logger.Info("channel opened", "ch", ch)
	qName := rand.Text()
	q, err := ch.QueueDeclare(
		qName, // name
		false, // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, body := range []string{"for ack", "for nack"} {
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
		}
	}
	time.Sleep(10 * time.Millisecond) // Wait for the message to be delivered

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	d, err := ch.ConsumeWithContext(ctx, q.Name, "", false, false, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	msgs := 0
	for {
		msg, ok := <-d
		if !ok {
			break
		}
		msgs++
		logger.Info("message received", "message", msg)
		if strings.Contains(string(msg.Body), "for ack") {
			logger.Info("ack")
			if err := ch.Ack(msg.DeliveryTag, false); err != nil {
				t.Error(err)
			}
		} else {
			logger.Info("nack")
			if err := ch.Nack(msg.DeliveryTag, false, true); err != nil {
				t.Error(err)
			}
			logger.Info("nack done")
		}
		if msgs == 2 {
			break
		}
	}
	cancel() // channel closed
	conn.Close()

	{
		conn2 := mustTestConn(t)
		ch2, err := conn2.Channel()
		if err != nil {
			t.Fatal(err)
		}
		msg, ok, err := ch2.Get(q.Name, true)
		if err != nil {
			t.Error(err)
		}
		if !ok {
			t.Errorf("message not found")
		}
		logger.Info("message received", "message", msg)
		if strings.Contains(string(msg.Body), "for nack") {
			// ok
		} else {
			t.Errorf("unexpected message: %s", string(msg.Body))
		}
		if _, err := ch2.QueueDelete(q.Name, false, false, false); err != nil {
			t.Error(err)
		}
	}
}
