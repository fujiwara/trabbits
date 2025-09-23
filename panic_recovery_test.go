package trabbits_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/fujiwara/trabbits"
	"github.com/fujiwara/trabbits/config"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecoverFromPanic(t *testing.T) {
	// Create a buffer to capture log output
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Create a server instance to get metrics
	cfg := &config.Config{}
	server := trabbits.NewServer(cfg, "/tmp/test-api.sock")
	metrics := server.Metrics()

	functionName := "test_function"
	panicMessage := "test panic message"

	// Get initial metric value
	initialCount := testutil.ToFloat64(metrics.PanicRecoveries.WithLabelValues(functionName))

	// Test panic recovery
	func() {
		defer trabbits.RecoverFromPanic(logger, functionName, metrics)
		panic(panicMessage)
	}()

	// Verify metric was incremented
	finalCount := testutil.ToFloat64(metrics.PanicRecoveries.WithLabelValues(functionName))
	expectedCount := initialCount + 1

	if finalCount != expectedCount {
		t.Errorf("Expected panic recovery metric to be %f, got %f", expectedCount, finalCount)
	}

	// Verify log output contains expected information
	logOutput := logBuf.String()

	if !strings.Contains(logOutput, "panic recovered") {
		t.Error("Log output should contain 'panic recovered'")
	}

	if !strings.Contains(logOutput, functionName) {
		t.Errorf("Log output should contain function name '%s'", functionName)
	}

	if !strings.Contains(logOutput, panicMessage) {
		t.Errorf("Log output should contain panic message '%s'", panicMessage)
	}

	if !strings.Contains(logOutput, "stack") {
		t.Error("Log output should contain stack trace")
	}
}

func TestRecoverFromPanicNoPanic(t *testing.T) {
	// Create a buffer to capture log output
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Create a server instance to get metrics
	cfg := &config.Config{}
	server := trabbits.NewServer(cfg, "/tmp/test-api-2.sock")
	metrics := server.Metrics()

	functionName := "test_function_no_panic"

	// Get initial metric value
	initialCount := testutil.ToFloat64(metrics.PanicRecoveries.WithLabelValues(functionName))

	// Test normal execution (no panic)
	func() {
		defer trabbits.RecoverFromPanic(logger, functionName, metrics)
		// Normal execution, no panic
	}()

	// Verify metric was NOT incremented
	finalCount := testutil.ToFloat64(metrics.PanicRecoveries.WithLabelValues(functionName))

	if finalCount != initialCount {
		t.Errorf("Expected panic recovery metric to remain %f, got %f", initialCount, finalCount)
	}

	// Verify no panic-related log output
	logOutput := logBuf.String()

	if strings.Contains(logOutput, "panic recovered") {
		t.Error("Log output should not contain 'panic recovered' when no panic occurs")
	}
}

func TestRecoverFromPanicDifferentFunctions(t *testing.T) {
	// Create a buffer to capture log output
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Create a server instance to get metrics
	cfg := &config.Config{}
	server := trabbits.NewServer(cfg, "/tmp/test-api-3.sock")
	metrics := server.Metrics()

	function1 := "handleConnection"
	function2 := "runHeartbeat"

	// Get initial metric values
	initialCount1 := testutil.ToFloat64(metrics.PanicRecoveries.WithLabelValues(function1))
	initialCount2 := testutil.ToFloat64(metrics.PanicRecoveries.WithLabelValues(function2))

	// Test panic in function1
	func() {
		defer trabbits.RecoverFromPanic(logger, function1, metrics)
		panic("panic in function1")
	}()

	// Test panic in function2
	func() {
		defer trabbits.RecoverFromPanic(logger, function2, metrics)
		panic("panic in function2")
	}()

	// Verify metrics were incremented correctly for each function
	finalCount1 := testutil.ToFloat64(metrics.PanicRecoveries.WithLabelValues(function1))
	finalCount2 := testutil.ToFloat64(metrics.PanicRecoveries.WithLabelValues(function2))

	if finalCount1 != initialCount1+1 {
		t.Errorf("Expected %s metric to be %f, got %f", function1, initialCount1+1, finalCount1)
	}

	if finalCount2 != initialCount2+1 {
		t.Errorf("Expected %s metric to be %f, got %f", function2, initialCount2+1, finalCount2)
	}

	// Verify both function names appear in logs
	logOutput := logBuf.String()

	if !strings.Contains(logOutput, function1) {
		t.Errorf("Log output should contain function name '%s'", function1)
	}

	if !strings.Contains(logOutput, function2) {
		t.Errorf("Log output should contain function name '%s'", function2)
	}
}
