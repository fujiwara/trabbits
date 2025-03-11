package trabbits_test

import (
	"context"
	"fmt"
	"log/slog"
	"net"
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
	go trabbits.BootProxy(ctx, listener)
	time.Sleep(100 * time.Millisecond) // Wait for the server to start
	return nil
}

var (
	serverCtx, cancel = context.WithCancel(context.Background())
	logger            *slog.Logger
)

func TestMain(m *testing.M) {
	handler := slog.Default().Handler()
	logger = slog.New(handler).With("test", true)

	runTestProxy(serverCtx)
	defer cancel()
	m.Run()
}

func TestProxyConnect(t *testing.T) {
	conn, err := amqp091.Dial(fmt.Sprintf("amqp://guest:guest@127.0.0.1:%d/", testProxyPort))
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("connected", "conn", conn)
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("channel opened", "ch", ch)
	defer ch.Close()
	defer conn.Close()
}
