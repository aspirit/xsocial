package otelutil

import (
	"context"
	"testing"
)

// TestCount_NilProviderIsNoop ensures business instrumentation is safe to call
// when telemetry was never initialized (e.g. in unit tests or a CLI tool).
func TestCount_NilProviderIsNoop(t *testing.T) {
	Reset() // DefaultTelemetry = nil
	// Must not panic.
	Count(context.Background(), "test_counter_total", 1)
}
