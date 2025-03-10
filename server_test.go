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

func runTestServer() error {
	listener, err := net.Listen("tcp", "localhost:0") // Listen on a ephemeral port
	if err != nil {
		slog.Error("Failed to start test server", "error", err)
		return err
	}
	testServerPort = listener.Addr().(*net.TCPAddr).Port
	go trabbits.BootServer(context.Background(), listener)
	time.Sleep(100 * time.Millisecond) // Wait for the server to start
	return nil
}

func TestMain(m *testing.M) {
	runTestServer()
	m.Run()
}

func TestServerConnect(t *testing.T) {
	conn, err := amqp091.Dial(fmt.Sprintf("amqp://guest:guest@127.0.0.1:%d/", testServerPort))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
}
