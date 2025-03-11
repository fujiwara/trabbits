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

var testServerPort int

func runTestServer(ctx context.Context) error {
	listener, err := net.Listen("tcp", "localhost:0") // Listen on a ephemeral port
	if err != nil {
		slog.Error("Failed to start test server", "error", err)
		return err
	}
	testServerPort = listener.Addr().(*net.TCPAddr).Port
	go trabbits.BootServer(ctx, listener)
	time.Sleep(100 * time.Millisecond) // Wait for the server to start
	return nil
}

var (
	serverCtx, cancel = context.WithCancel(context.Background())
	logger            *slog.Logger
)

func TestMain(m *testing.M) {
	handler := slog.Default().Handler()
	logger = slog.New(handler).With("client", "test")

	runTestServer(serverCtx)
	defer cancel()
	m.Run()
}

func TestServerConnect(t *testing.T) {
	conn, err := amqp091.Dial(fmt.Sprintf("amqp://guest:guest@127.0.0.1:%d/", testServerPort))
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
