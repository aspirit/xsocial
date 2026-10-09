package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aspirit/xsocial/pkg/cache"
	"github.com/aspirit/xsocial/pkg/logger"
)

// fakeCache is an in-memory cache.Cache with an atomic IncrWithTTL, used to test
// CacheLimiter without a real Redis.
type fakeCache struct {
	mu     sync.Mutex
	counts map[string]int64
}

func newFakeCache() *fakeCache { return &fakeCache{counts: make(map[string]int64)} }

func (f *fakeCache) IncrWithTTL(_ context.Context, key string, ttl time.Duration) (int64, time.Duration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counts[key]++

	return f.counts[key], ttl, nil
}

func (f *fakeCache) Get(context.Context, string) (string, error)           { return "", cache.ErrNotFound }
func (f *fakeCache) Set(context.Context, string, any, time.Duration) error { return nil }
func (f *fakeCache) Del(context.Context, ...string) error                  { return nil }
func (f *fakeCache) Ping(context.Context) error                            { return nil }
func (f *fakeCache) Close() error                                          { return nil }

func TestCacheLimiter_Allow_BlocksOverLimit(t *testing.T) {
	lim, err := NewCacheLimiter(newFakeCache(), logger.NewNopLogger())
	if err != nil {
		t.Fatalf("NewCacheLimiter: %v", err)
	}
	ctx := context.Background()
	const (
		key    = "k"
		limit  = 3
		window = time.Minute
	)

	for i := 1; i <= limit; i++ {
		allowed, _, err := lim.Allow(ctx, key, limit, window)
		if err != nil {
			t.Fatalf("Allow #%d: %v", i, err)
		}
		if !allowed {
			t.Fatalf("request %d should be allowed (within limit)", i)
		}
	}

	allowed, retryAfter, err := lim.Allow(ctx, key, limit, window)
	if err != nil {
		t.Fatalf("Allow over limit: %v", err)
	}
	if allowed {
		t.Fatal("request over the limit should be blocked")
	}
	if retryAfter <= 0 {
		t.Fatalf("retryAfter should be positive, got %d", retryAfter)
	}
}

// TestCacheLimiter_Allow_ConcurrentAtomic verifies the atomic counter does not
// over-admit under concurrency (the bug the Get+Set version had).
func TestCacheLimiter_Allow_ConcurrentAtomic(t *testing.T) {
	lim, err := NewCacheLimiter(newFakeCache(), logger.NewNopLogger())
	if err != nil {
		t.Fatalf("NewCacheLimiter: %v", err)
	}
	const (
		key   = "concurrent"
		limit = 50
		total = 500
	)

	var mu sync.Mutex
	allowedCount := 0
	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed, _, _ := lim.Allow(context.Background(), key, limit, time.Minute)
			if allowed {
				mu.Lock()
				allowedCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if allowedCount != limit {
		t.Fatalf("expected exactly %d allowed under concurrency, got %d", limit, allowedCount)
	}
}
