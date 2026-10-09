package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMemoryLimiter_Allow_FirstRequest(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryLimiter()
	defer m.Stop()

	allowed, retryAfter, err := m.Allow(ctx, "key1", 2, time.Minute)
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if !allowed {
		t.Error("first request should be allowed")
	}
	if retryAfter != 0 {
		t.Errorf("retryAfter want 0, got %d", retryAfter)
	}
}

func TestMemoryLimiter_Allow_WithinLimit(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryLimiter()
	defer m.Stop()

	for i := 0; i < 3; i++ {
		allowed, _, err := m.Allow(ctx, "key1", 3, time.Minute)
		if err != nil {
			t.Fatalf("Allow: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}
}

func TestMemoryLimiter_Allow_OverLimit(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryLimiter()
	defer m.Stop()

	limit := 3
	window := time.Minute // long window so we are still inside when we reject
	key := "key1"

	for i := 0; i < limit; i++ {
		allowed, _, _ := m.Allow(ctx, key, limit, window)
		if !allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}
	// next one should be rejected
	allowed, retryAfter, err := m.Allow(ctx, key, limit, window)
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if allowed {
		t.Error("request over limit should be rejected")
	}
	if retryAfter < 0 {
		t.Errorf("retryAfter should be non-negative, got %d", retryAfter)
	}
}

func TestMemoryLimiter_Allow_WindowExpiry(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryLimiter()
	defer m.Stop()

	limit := 2
	window := 50 * time.Millisecond
	key := "key1"

	// use up limit
	for i := 0; i < limit; i++ {
		_, _, _ = m.Allow(ctx, key, limit, window)
	}
	allowed, _, _ := m.Allow(ctx, key, limit, window)
	if allowed {
		t.Error("over limit should be rejected")
	}
	// wait for window to expire
	time.Sleep(window + 10*time.Millisecond)
	allowed, _, err := m.Allow(ctx, key, limit, window)
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if !allowed {
		t.Error("after window expiry, request should be allowed again")
	}
}

func TestMemoryLimiter_Allow_Concurrent(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryLimiter()
	defer m.Stop()

	var wg sync.WaitGroup
	allowedCount := 0
	var mu sync.Mutex
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed, _, _ := m.Allow(ctx, "concurrent_key", 5, time.Minute)
			if allowed {
				mu.Lock()
				allowedCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if allowedCount != 5 {
		t.Errorf("expected exactly 5 allowed, got %d", allowedCount)
	}
}
