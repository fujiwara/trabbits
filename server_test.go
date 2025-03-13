package trabbits_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fujiwara/trabbits"
	"github.com/google/go-cmp/cmp"
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
	defer conn.Close()
	ch := mustTestChannel(t, conn)
	defer ch.Close()
}

func TestProxyChannel(t *testing.T) {
	conn := mustTestConn(t)
	defer conn.Close()
	logger.Info("connected", "conn", conn)

	for i := 0; i < 3; i++ {
		ch := mustTestChannel(t, conn)
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
	defer conn.Close()
	ch := mustTestChannel(t, conn)
	defer ch.Close()

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
}

func mustTestConn(t *testing.T) *amqp091.Connection {
	conn, err := amqp091.Dial(fmt.Sprintf("amqp://admin:admin@127.0.0.1:%d/", testProxyPort))
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("connected", "conn", conn.Properties)
	return conn
}

func mustTestChannel(t *testing.T, conn *amqp091.Connection) *amqp091.Channel {
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("channel opened")
	return ch
}

func TestProxyAckNack(t *testing.T) {
	conn := mustTestConn(t)
	defer conn.Close()
	ch := mustTestChannel(t, conn)
	defer ch.Close()

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

func TestProxyQos(t *testing.T) {
	conn := mustTestConn(t)
	defer conn.Close()
	ch := mustTestChannel(t, conn)
	defer ch.Close()

	qName := rand.Text()
	if _, err := ch.QueueDeclare(
		qName, // name
		false, // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	); err != nil {
		t.Fatal(err)
	}
	for i := range 5 {
		if err := ch.Publish(
			"",    // exchange
			qName, // routing key
			false, // mandatory
			false, // immediate
			amqp091.Publishing{
				ContentType: "text/plain",
				Body:        []byte(fmt.Sprintf("%d-%s", i, rand.Text())),
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	wg := sync.WaitGroup{}
	processed := sync.Map{}
	for i := range 3 {
		wg.Add(1)
		// run consumers concurrently
		go func(id int) {
			defer wg.Done()
			conn := mustTestConn(t)
			defer conn.Close()
			ch, _ := conn.Channel()
			defer ch.Close()
			if err := ch.Qos(1, 0, false); err != nil { // prefetch only 1 message
				t.Error(err)
				return
			}
			ctx, cancel := context.WithTimeout(t.Context(), 1*time.Second)
			defer cancel()
			d, err := ch.ConsumeWithContext(ctx, qName, "", false, false, false, false, nil)
			if err != nil {
				t.Error(err)
				return
			}
			for {
				select {
				case <-ctx.Done():
					return
				case msg := <-d:
					logger.Info("message received", "body", string(msg.Body), "id", id)
					time.Sleep(100 * time.Millisecond) // simulate processing
					if err := ch.Ack(msg.DeliveryTag, false); err != nil {
						t.Error(err)
					}
					processed.Store(id, true)
				}
			}
		}(i)
	}
	wg.Wait()
	processedIDs := []int{}
	processed.Range(func(k, v interface{}) bool {
		processedIDs = append(processedIDs, k.(int))
		return true
	})
	sort.Ints(processedIDs)
	t.Logf("processed IDs: %v", processedIDs)
	if cmp.Diff(processedIDs, []int{0, 1, 2}) != "" {
		t.Errorf("unexpected processed IDs: %v", processedIDs)
	}
}

func TestProxyExchangeDirect(t *testing.T) {
	conn := mustTestConn(t)
	defer conn.Close()
	ch := mustTestChannel(t, conn)
	defer ch.Close()

	var (
		exchange    = "test-exchange"
		queue       = "test-queue"
		routingKey  = "test-routing-key"
		testMessage = "test message" + rand.Text()
	)

	err := ch.ExchangeDeclare(
		exchange, // name
		"direct", // kind
		false,    // durable
		false,    // auto-delete
		false,    // internal
		false,    // no-wait
		nil,      // arguments
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ch.QueueDeclare(queue, false, false, false, false, nil); err != nil {
		t.Fatal(err)
	}
	if err := ch.QueueBind(queue, routingKey, exchange, false, nil); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 1*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	var gotMessage string
	go func() {
		defer wg.Done()
		d, err := ch.ConsumeWithContext(ctx, queue, "", false, false, false, false, nil)
		if err != nil {
			t.Error(err)
			return
		}
		msg := <-d
		logger.Info("message received", "message", msg)
		if err := ch.Ack(msg.DeliveryTag, false); err != nil {
			t.Error(err)
		}
		gotMessage = string(msg.Body)
	}()

	// send message by another connection
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn := mustTestConn(t)
		defer conn.Close()
		ch := mustTestChannel(t, conn)
		defer ch.Close()
		if err := ch.Publish(
			exchange,   // exchange
			routingKey, // routing key
			false,      // mandatory
			false,      // immediate
			amqp091.Publishing{
				ContentType: "text/plain",
				Body:        []byte(testMessage),
			},
		); err != nil {
			t.Error(err)
		}
	}()
	wg.Wait()

	t.Logf("got message: %s", gotMessage)
	if gotMessage != testMessage {
		t.Errorf("unexpected message: %s", gotMessage)
	}
}
