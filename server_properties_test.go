package trabbits_test

import (
	"strings"
	"testing"

	rabbitmq "github.com/rabbitmq/amqp091-go"
)

func TestServerProperties(t *testing.T) {
	t.Parallel()
	if !testViaTrabbits {
		t.Skip("Skipping: connected to real RabbitMQ, not trabbits")
	}
	conn := mustTestConn(t)
	defer conn.Close()

	props := conn.Properties
	if got := props["product"]; got != "trabbits" {
		t.Errorf("product = %v, want trabbits", got)
	}
	version, ok := props["version"].(string)
	if !ok || version == "" {
		t.Errorf("version = %v, want a non-empty string", props["version"])
	}
	// Clients (e.g. perf-test) parse the version to detect broker features,
	// so it must be a bare version number without the "v" prefix.
	if strings.HasPrefix(version, "v") {
		t.Errorf("version = %q, must not have a v prefix", version)
	}
	caps, ok := props["capabilities"].(rabbitmq.Table)
	if !ok {
		t.Fatalf("capabilities = %v, want a table", props["capabilities"])
	}
	for _, name := range []string{"publisher_confirms", "basic.nack"} {
		if v, _ := caps[name].(bool); !v {
			t.Errorf("capability %s = %v, want true", name, caps[name])
		}
	}
}
