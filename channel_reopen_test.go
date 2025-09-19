package trabbits_test

import (
	"context"
	"testing"
	"time"

	rabbitmq "github.com/rabbitmq/amqp091-go"
)

func TestChannelReopenAfterClose(t *testing.T) {
	// Test with timeout to prevent hanging
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	conn := mustTestConn(t, "test_type", "channel_reopen")
	defer conn.Close()

	// First: Open channel 1
	t.Log("Opening channel 1 for the first time")
	ch1, err := conn.Channel()
	if err != nil {
		t.Fatalf("Failed to open channel 1: %v", err)
	}

	// Declare a queue to verify channel is working
	queueName := "test_channel_reopen"
	_, err = ch1.QueueDeclare(
		queueName,
		false, // durable
		true,  // auto-delete
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		t.Fatalf("Failed to declare queue on first channel: %v", err)
	}
	t.Log("Successfully declared queue on first channel")

	// Close channel 1
	t.Log("Closing channel 1")
	if err := ch1.Close(); err != nil {
		t.Fatalf("Failed to close channel 1: %v", err)
	}

	// Wait a moment to ensure close is processed
	time.Sleep(100 * time.Millisecond)

	// Try to reopen channel (should get a new channel ID internally)
	t.Log("Attempting to open channel again after close")

	// Create channel with timeout using context
	done := make(chan struct{})
	var ch2 *rabbitmq.Channel
	var openErr error

	go func() {
		ch2, openErr = conn.Channel()
		close(done)
	}()

	select {
	case <-ctx.Done():
		t.Fatal("Timeout: Channel reopen hung after close (this demonstrates the bug)")
	case <-done:
		if openErr != nil {
			t.Fatalf("Failed to reopen channel: %v", openErr)
		}
	}

	// Try to use the reopened channel
	t.Log("Testing reopened channel with queue operation")
	_, err = ch2.QueueDeclare(
		queueName+"_2",
		false, // durable
		true,  // auto-delete
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		t.Fatalf("Failed to declare queue on reopened channel: %v", err)
	}
	t.Log("Successfully declared queue on reopened channel")

	// Clean up
	if err := ch2.Close(); err != nil {
		t.Logf("Failed to close channel 2: %v", err)
	}
}

func TestMultipleChannelOpenCloseSequence(t *testing.T) {
	// Test with timeout to prevent hanging
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	conn := mustTestConn(t, "test_type", "channel_sequence")
	defer conn.Close()

	const iterations = 1000

	// Test sequence: open, close, open, queue operation, close
	for i := 0; i < iterations; i++ {
		if i%100 == 0 {
			t.Logf("Progress: %d/%d iterations completed", i, iterations)
		}

		done := make(chan struct{})
		var ch *rabbitmq.Channel
		var err error

		go func() {
			ch, err = conn.Channel()
			close(done)
		}()

		select {
		case <-ctx.Done():
			t.Fatalf("Iteration %d: Timeout opening channel (bug reproduced)", i+1)
		case <-done:
			if err != nil {
				t.Fatalf("Iteration %d: Failed to open channel: %v", i+1, err)
			}
		}

		// Perform a queue operation
		queueName := "test_channel_sequence"
		_, err = ch.QueueDeclare(
			queueName,
			false, // durable
			true,  // auto-delete
			false, // exclusive
			false, // no-wait
			nil,   // arguments
		)
		if err != nil {
			t.Fatalf("Iteration %d: Failed to declare queue: %v", i+1, err)
		}

		// Publish a message
		err = ch.PublishWithContext(
			ctx,
			"",        // exchange
			queueName, // routing key
			false,     // mandatory
			false,     // immediate
			rabbitmq.Publishing{
				Body: []byte("test message"),
			},
		)
		if err != nil {
			t.Fatalf("Iteration %d: Failed to publish message: %v", i+1, err)
		}

		// Close the channel
		if err := ch.Close(); err != nil {
			t.Fatalf("Iteration %d: Failed to close channel: %v", i+1, err)
		}
	}

	t.Log("All iterations completed successfully")
}
