package trabbits_test

import (
	"testing"
	"time"

	"github.com/fujiwara/trabbits"
)

func TestProbeLog(t *testing.T) {
	// Create a proxy with probe channel
	probeChan := make(chan trabbits.ProbeLog, 10)
	proxy := &trabbits.Proxy{}
	proxy.SetProbeChan(probeChan)

	t.Run("SendProbeLog", func(t *testing.T) {
		// Send a simple probe log
		proxy.SendProbeLog("test message")

		// Check if the log was received
		select {
		case log := <-probeChan:
			if log.Message != "test message" {
				t.Errorf("expected message 'test message', got '%s'", log.Message)
			}
			if log.Timestamp.IsZero() {
				t.Error("timestamp should not be zero")
			}
			if len(log.AttrsMap()) != 0 {
				t.Errorf("expected no attributes, got %v", log.AttrsMap())
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("probe log was not received")
		}
	})

	t.Run("SendProbeLogWithAttributes", func(t *testing.T) {
		// Send probe log with attributes
		proxy.SendProbeLog("test with attrs", "key1", "value1", "key2", 42, "key3", true)

		// Check if the log was received with attributes
		select {
		case log := <-probeChan:
			if log.Message != "test with attrs" {
				t.Errorf("expected message 'test with attrs', got '%s'", log.Message)
			}

			attrs := log.AttrsMap()
			if len(attrs) != 3 {
				t.Errorf("expected 3 attributes, got %d", len(attrs))
			}

			if v, ok := attrs["key1"]; !ok || v != "value1" {
				t.Errorf("expected key1='value1', got %v", v)
			}
			if v, ok := attrs["key2"]; !ok || v != 42 {
				t.Errorf("expected key2=42, got %v", v)
			}
			if v, ok := attrs["key3"]; !ok || v != true {
				t.Errorf("expected key3=true, got %v", v)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("probe log with attributes was not received")
		}
	})

	t.Run("SendProbeLogWithOddAttributes", func(t *testing.T) {
		// Send probe log with odd number of attributes (last one should be ignored)
		proxy.SendProbeLog("odd attrs", "key1", "value1", "key2")

		select {
		case log := <-probeChan:
			attrs := log.AttrsMap()
			if len(attrs) != 1 {
				t.Errorf("expected 1 attribute (key2 should be ignored), got %d", len(attrs))
			}
			if v, ok := attrs["key1"]; !ok || v != "value1" {
				t.Errorf("expected key1='value1', got %v", v)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("probe log was not received")
		}
	})

	t.Run("SendProbeLogWhenChannelFull", func(t *testing.T) {
		// Fill the channel
		smallChan := make(chan trabbits.ProbeLog, 1)
		proxyWithSmallChan := &trabbits.Proxy{}
		proxyWithSmallChan.SetProbeChan(smallChan)

		// Send first log (should succeed)
		proxyWithSmallChan.SendProbeLog("first")

		// Send second log (should be dropped without blocking)
		done := make(chan bool)
		go func() {
			proxyWithSmallChan.SendProbeLog("second")
			done <- true
		}()

		select {
		case <-done:
			// Good, didn't block
		case <-time.After(100 * time.Millisecond):
			t.Error("SendProbeLog blocked when channel was full")
		}

		// Verify only second message is in the channel (first was removed)
		select {
		case log := <-smallChan:
			if log.Message != "second" {
				t.Errorf("expected 'second', got '%s'", log.Message)
			}
		default:
			t.Error("expected one message in channel")
		}
	})

	t.Run("SendProbeLogWithNilChannel", func(t *testing.T) {
		// Create proxy without probe channel
		proxyWithoutChan := &trabbits.Proxy{}

		// This should not panic
		proxyWithoutChan.SendProbeLog("should not panic")
		// If we get here, test passed
	})

	t.Run("AttrsMapWithComplexTypes", func(t *testing.T) {
		type testStruct struct {
			Field1 string
			Field2 int
		}

		proxy.SendProbeLog("complex types",
			"string", "test",
			"int", 123,
			"float", 3.14,
			"bool", false,
			"struct", testStruct{Field1: "test", Field2: 456},
		)

		select {
		case log := <-probeChan:
			attrs := log.AttrsMap()
			if len(attrs) != 5 {
				t.Errorf("expected 5 attributes, got %d", len(attrs))
			}

			// Verify struct was stored correctly
			if v, ok := attrs["struct"]; ok {
				if s, ok := v.(testStruct); ok {
					if s.Field1 != "test" || s.Field2 != 456 {
						t.Errorf("struct fields incorrect: %+v", s)
					}
				} else {
					t.Errorf("struct type assertion failed: %T", v)
				}
			} else {
				t.Error("struct attribute not found")
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("probe log was not received")
		}
	})
}
