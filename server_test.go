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
	"sync/atomic"
	"testing"
	"time"

	"github.com/fujiwara/trabbits"
	"github.com/google/go-cmp/cmp"
	rabbitmq "github.com/rabbitmq/amqp091-go"
)

var testProxyPort = 5672
var testViaTrabbits = true
var testAPISock string

func runTestProxy(ctx context.Context) error {
	slog.Info("starting test server")
	cfg, err := trabbits.LoadConfig(ctx, "testdata/config.json")
	if err != nil {
		panic("failed to load config: " + err.Error())
	}
	trabbits.StoreConfig(cfg)

	if b, _ := strconv.ParseBool(os.Getenv("TEST_RABBITMQ")); b {
		slog.Info("skipping test server, use real RabbitMQ directly")
		testViaTrabbits = false
		return nil
	}

	listener, err := net.Listen("tcp", "localhost:0") // Listen on a ephemeral port
	if err != nil {
		slog.Error("Failed to start test server", "error", err)
		return err
	}
	testProxyPort = listener.Addr().(*net.TCPAddr).Port
	if os.Getenv("TEST_PROXY_PORT") != "" {
		testProxyPort, _ = strconv.Atoi(os.Getenv("TEST_PROXY_PORT"))
	}
	trabbits.SetReadTimeout(1 * time.Second) // for testing
	go trabbits.Boot(ctx, listener)
	return nil
}

func runTestAPI(ctx context.Context) error {
	tmpfile, err := os.CreateTemp("", "trabbits-test-api-sock-")
	if err != nil {
		slog.Error("failed to create temp file", "error", err)
	}
	testAPISock = tmpfile.Name()
	os.Remove(testAPISock) // trabbits will re create it
	go trabbits.RunAPIServer(ctx, &trabbits.CLI{APISocket: testAPISock})
	return nil
}

var (
	serverCtx, cancel = context.WithCancel(context.Background())
	logger            *slog.Logger
)

func TestMain(m *testing.M) {
	debug, _ := strconv.ParseBool(os.Getenv("DEBUG"))
	if debug {
		trabbits.SetupLogger(slog.LevelDebug)
	} else {
		trabbits.SetupLogger(slog.LevelInfo)
	}
	handler := slog.Default().Handler()
	logger = slog.New(handler).With("test", true)

	// escape if the test is taking too long
	time.AfterFunc(60*time.Second, func() {
		panic("timeout")
	})

	runTestProxy(serverCtx)
	runTestAPI(serverCtx)
	time.Sleep(100 * time.Millisecond) // Wait for the server to start
	defer cancel()
	m.Run()

	// dumpMetrics()
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
		rabbitmq.Publishing{
			ContentType: "text/plain",
			Body:        []byte(body),
		},
	); err != nil {
		t.Fatal(err)
	} else {
		logger.Info("message published")
	}

	time.Sleep(10 * time.Millisecond) // Wait for the message to be delivered

	m, ok, err := ch.Get(q.Name, false)
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
}

func TestProxyPublishAutoQueueNaming(t *testing.T) {
	conn := mustTestConn(t)
	ch := mustTestChannel(t, conn)

	q, err := ch.QueueDeclare(
		"",    // auto-named queue
		false, // durable
		true,  // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		t.Fatal(err)
	}
	if q.Name == "" {
		t.Error("empty queue name")
	}
	logger.Info("queue declared (auto naming)", "queue", q.Name)

	wg := sync.WaitGroup{}
	wg.Add(1)
	// consume the message
	go func() {
		defer wg.Done()
		time.Sleep(100 * time.Millisecond) // Wait for the message to be delivered
		ctx, cancel := context.WithTimeout(t.Context(), 1*time.Second)
		defer cancel()
		d, err := ch.ConsumeWithContext(ctx, q.Name, "", false, false, false, false, nil)
		if err != nil {
			t.Error(err)
			return
		}
		msg := <-d
		logger.Info("message received", "message", msg)
		if string(msg.Body) != "hello "+q.Name {
			t.Errorf("unexpected message: %s", string(msg.Body))
		}
		if err := ch.Ack(msg.DeliveryTag, false); err != nil {
			t.Error(err)
		}
	}()
	// publish the message
	ch.Publish(
		"",     // exchange
		q.Name, // routing key
		false,  // mandatory
		false,  // immediate
		rabbitmq.Publishing{
			ContentType: "text/plain",
			Body:        []byte("hello " + q.Name),
		},
	)
	wg.Wait()
	ch.Close()
	conn.Close()

	time.Sleep(100 * time.Millisecond) // Wait for the temporary queue to be deleted

	conn2 := mustTestConn(t)
	defer conn2.Close()
	ch2 := mustTestChannel(t, conn2)
	defer ch2.Close()
	if _, _, err := ch2.Get(q.Name, true); err == nil {
		t.Errorf("queue should be deleted")
	} else {
		logger.Info("queue deleted", "queue", q.Name, "error", err)
	}
}

func TestProxyPublishPurgeGet(t *testing.T) {
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
		rabbitmq.Publishing{
			ContentType: "text/plain",
			Body:        []byte(body),
		},
	); err != nil {
		t.Fatal(err)
	} else {
		logger.Info("message published")
	}

	if _, err := ch.QueuePurge(q.Name, false); err != nil {
		t.Error(err)
	}

	time.Sleep(10 * time.Millisecond) // Wait for the message to be delivered

	_, ok, err := ch.Get(q.Name, true)
	if err != nil {
		t.Error(err)
	}
	if ok {
		t.Errorf("message should not be purged")
	}
}

func mustTestConn(t *testing.T) *rabbitmq.Connection {
	conn, err := rabbitmq.Dial(fmt.Sprintf("amqp://admin:admin@127.0.0.1:%d/", testProxyPort))
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("connected", "conn", conn.Properties)
	return conn
}

func mustTestChannel(t *testing.T, conn *rabbitmq.Connection) *rabbitmq.Channel {
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
			rabbitmq.Publishing{
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
	defer func() {
		ch.QueueDelete(qName, false, false, false)
	}()
	var messages = 5
	var consumers = 3
	for i := range messages {
		if err := ch.Publish(
			"",    // exchange
			qName, // routing key
			false, // mandatory
			false, // immediate
			rabbitmq.Publishing{
				ContentType: "text/plain",
				Body:        []byte(fmt.Sprintf("%d-%s", i, rand.Text())),
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	wg := sync.WaitGroup{}
	processed := sync.Map{}
	for i := range consumers {
		wg.Add(1)
		// run consumers concurrently
		go func(id int) {
			defer wg.Done()
			conn := mustTestConn(t)
			defer conn.Close()
			ch, _ := conn.Channel()
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
					logger.Info("context done", "id", id)
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
	var expected = []int{}
	for i := range consumers {
		expected = append(expected, i)
	}
	t.Logf("processed IDs: %v expected IDs: %v", processedIDs, expected)
	if cmp.Diff(processedIDs, expected) != "" {
		t.Errorf("unexpected processed IDs: %v", processedIDs)
	}
}

func TestProxyExchangeDirect(t *testing.T) {
	conn := mustTestConn(t)
	defer conn.Close()
	ch := mustTestChannel(t, conn)
	defer ch.Close()

	var (
		exchange    = "test-exchange-direct"
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
			rabbitmq.Publishing{
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
		t.Errorf("unexpected message: %s expected: %s", gotMessage, testMessage)
	}
}

func TestProxyExchangeBroadcast(t *testing.T) {
	conn := mustTestConn(t)
	defer conn.Close()
	ch := mustTestChannel(t, conn)
	defer ch.Close()

	var (
		exchange         = "test-exchange-topic"
		queuePrefix      = "test-queue."
		routingKeyPrefix = "test-routing-key."
		testMessage      = "test message" + rand.Text()
	)

	err := ch.ExchangeDeclare(
		exchange, // name
		"topic",  // kind
		false,    // durable
		false,    // auto-delete
		false,    // internal
		false,    // no-wait
		nil,      // arguments
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var received int64
	consumers := 3
	bound := make(chan struct{}, consumers)
	for i := range consumers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			conn := mustTestConn(t)
			defer conn.Close()
			ch := mustTestChannel(t, conn)
			defer ch.Close()
			queue, err := ch.QueueDeclare(fmt.Sprintf("%s%d", queuePrefix, id), false, false, false, false, nil)
			if err != nil {
				t.Error(err)
				return
			}
			key := routingKeyPrefix + "*"
			logger.Info("queue bind", "queue", queue.Name, "key", key)
			if err := ch.QueueBind(queue.Name, key, exchange, false, nil); err != nil {
				t.Error(err)
				return
			}
			bound <- struct{}{}
			d, err := ch.ConsumeWithContext(ctx, queue.Name, "", false, false, false, false, nil)
			if err != nil {
				t.Error(err)
				return
			}
			for msg := range d {
				logger.Info("message received", "queue", queue.Name, "message", string(msg.Body))
				if string(msg.Body) != testMessage {
					t.Errorf("unexpected message: %s by %d", string(msg.Body), id)
				}
				if err := ch.Ack(msg.DeliveryTag, false); err != nil {
					t.Error(err)
				}
				atomic.AddInt64(&received, 1)
				break // 1 message per queue
			}
			// cleanup, ignore errors
			ch.QueueUnbind(queue.Name, key, exchange, nil)
			ch.QueueDelete(queue.Name, false, false, false)
		}(i)
	}

	// wait for all consumers to be ready
	for range consumers {
		<-bound
	}

	// send message
	key := routingKeyPrefix + "xxx"
	logger.Info("publish message", "message", testMessage, "exchange", exchange, "routingKey", key)
	if err := ch.Publish(
		exchange, // exchange
		key,      // routing key
		false,    // mandatory
		false,    // immediate
		rabbitmq.Publishing{
			ContentType: "text/plain",
			Body:        []byte(testMessage),
		},
	); err != nil {
		t.Fatal(err)
	}
	wg.Wait()

	logger.Info("messages should be received by all consumers")
	if atomic.LoadInt64(&received) != int64(consumers) {
		t.Errorf("unexpected received: %d", received)
	}
}

func TestProxyExchangeTopic(t *testing.T) {
	conn := mustTestConn(t)
	defer conn.Close()
	ch := mustTestChannel(t, conn)
	defer ch.Close()

	var (
		exchange         = "test-exchange-topic"
		queuePrefix      = "test-queue."
		routingKeyPrefix = "test-routing-key."
		testMessage      = "test message" + rand.Text()
	)

	err := ch.ExchangeDeclare(
		exchange, // name
		"topic",  // kind
		false,    // durable
		false,    // auto-delete
		false,    // internal
		false,    // no-wait
		nil,      // arguments
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var received int64
	consumers := 3
	bound := make(chan struct{}, consumers)
	for i := range consumers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			conn := mustTestConn(t)
			defer conn.Close()
			ch := mustTestChannel(t, conn)
			defer ch.Close()
			queue, err := ch.QueueDeclare(fmt.Sprintf("%s%d", queuePrefix, id), false, false, false, false, nil)
			if err != nil {
				t.Error(err)
				return
			}
			key := routingKeyPrefix + strconv.Itoa(id) // 0, 1, 2
			logger.Info("queue bind", "queue", queue.Name, "key", key)
			if err := ch.QueueBind(queue.Name, key, exchange, false, nil); err != nil {
				t.Error(err)
				return
			}
			bound <- struct{}{}
			d, err := ch.ConsumeWithContext(ctx, queue.Name, "", false, false, false, false, nil)
			if err != nil {
				t.Error(err)
				return
			}
			for msg := range d {
				logger.Info("message received", "queue", queue.Name, "message", string(msg.Body))
				if string(msg.Body) != testMessage+strconv.Itoa(id) {
					t.Errorf("unexpected message: %s by %d", string(msg.Body), id)
				}
				if err := ch.Ack(msg.DeliveryTag, false); err != nil {
					t.Error(err)
				}
				atomic.AddInt64(&received, 1)
				break // 1 message per queue
			}
			// cleanup, ignore errors
			ch.QueueUnbind(queue.Name, key, exchange, nil)
			ch.QueueDelete(queue.Name, false, false, false)
		}(i)
	}

	// wait for all consumers to be ready
	for range consumers {
		<-bound
	}

	// send message
	for i := range consumers {
		key := routingKeyPrefix + strconv.Itoa(i)
		logger.Info("publish message", "message", testMessage, "exchange", exchange, "routingKey", key)
		if err := ch.Publish(
			exchange, // exchange
			key,      // routing key
			false,    // mandatory
			false,    // immediate
			rabbitmq.Publishing{
				ContentType: "text/plain",
				Body:        []byte(testMessage + strconv.Itoa(i)),
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()

	logger.Info("messages should be received by all consumers")
	if atomic.LoadInt64(&received) != int64(consumers) {
		t.Errorf("unexpected received: %d", received)
	}
}

func TestSlowClient(t *testing.T) {
	conn := mustTestConn(t)
	defer conn.Close()
	time.Sleep(trabbits.GetReadTimeout() + 100*time.Millisecond)
	ch := mustTestChannel(t, conn)
	defer ch.Close()
}
