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
	conn, err := amqp091.Dial(fmt.Sprintf("amqp://admin:admin@127.0.0.1:%d/", testProxyPort))
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("connected", "conn", conn)
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("channel opened", "ch", ch)
}

func TestProxyPublish(t *testing.T) {
	conn, err := amqp091.Dial(fmt.Sprintf("amqp://admin:admin@127.0.0.1:%d/", testProxyPort))
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("connected", "conn", conn)
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("channel opened", "ch", ch)

	qName := "hello"
	/*
		// Purge the queue before publishing
		if n, err := ch.QueuePurge(qName, false); err != nil {
			t.Fatal(err)
		} else {
			t.Log("purged", n, "messages")
		}
	*/

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

	time.Sleep(100 * time.Millisecond) // Wait for the message to be delivered

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
