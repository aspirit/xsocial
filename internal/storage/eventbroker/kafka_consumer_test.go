package eventbroker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aspirit/xsocial/pkg/eventbroker"
	"github.com/aspirit/xsocial/pkg/logger"
)

var errFlaky = errors.New("flaky")

// flakyHandler fails the first failures calls, then succeeds.
type flakyHandler struct {
	failures int
	calls    int
}

func (h *flakyHandler) Handle(context.Context, *eventbroker.Message) error {
	h.calls++
	if h.calls <= h.failures {
		return errFlaky
	}

	return nil
}

func TestHandleWithRetry_AbsorbsTransientFailures(t *testing.T) {
	h := &flakyHandler{failures: 2}
	err := handleWithRetry(context.Background(), h, &eventbroker.Message{Topic: "t"}, logger.NewNopLogger(), 3, time.Millisecond)
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if h.calls != 3 {
		t.Fatalf("expected 3 attempts, got %d", h.calls)
	}
}

func TestHandleWithRetry_ReturnsPersistentFailure(t *testing.T) {
	h := &flakyHandler{failures: 10}
	err := handleWithRetry(context.Background(), h, &eventbroker.Message{Topic: "t"}, logger.NewNopLogger(), 2, time.Millisecond)
	if !errors.Is(err, errFlaky) {
		t.Fatalf("expected the handler error to surface, got %v", err)
	}
	if h.calls != 2 {
		t.Fatalf("expected exactly 2 attempts, got %d", h.calls)
	}
}

func TestHandleWithRetry_StopsOnCancel(t *testing.T) {
	h := &flakyHandler{failures: 10}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := handleWithRetry(ctx, h, &eventbroker.Message{Topic: "t"}, logger.NewNopLogger(), 5, time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context error, got %v", err)
	}
	if h.calls != 1 {
		t.Fatalf("expected a single attempt before honouring cancellation, got %d", h.calls)
	}
}
