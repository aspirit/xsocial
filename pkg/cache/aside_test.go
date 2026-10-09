package cache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aspirit/xsocial/pkg/cache"
)

// memCache is a minimal in-memory cache.Cache for tests.
type memCache struct {
	data map[string]string
}

func newMemCache() *memCache { return &memCache{data: map[string]string{}} }

func (m *memCache) Get(_ context.Context, key string) (string, error) {
	if v, ok := m.data[key]; ok {
		return v, nil
	}

	return "", cache.ErrNotFound
}

func (m *memCache) Set(_ context.Context, key string, value any, _ time.Duration) error {
	m.data[key] = value.(string)

	return nil
}

func (m *memCache) Del(_ context.Context, keys ...string) error {
	for _, k := range keys {
		delete(m.data, k)
	}

	return nil
}

func (m *memCache) IncrWithTTL(context.Context, string, time.Duration) (int64, time.Duration, error) {
	return 0, 0, nil
}

func (m *memCache) Ping(context.Context) error { return nil }
func (m *memCache) Close() error               { return nil }

func TestAside_MissThenHit(t *testing.T) {
	c := newMemCache()
	ctx := context.Background()
	loads := 0
	load := func(context.Context) (int, error) {
		loads++

		return 42, nil
	}

	// Miss: loads and caches.
	v, err := cache.Aside(ctx, c, "k", time.Minute, load)
	if err != nil || v != 42 {
		t.Fatalf("first: v=%d err=%v", v, err)
	}
	// Hit: served from cache, no second load.
	v, err = cache.Aside(ctx, c, "k", time.Minute, load)
	if err != nil || v != 42 {
		t.Fatalf("second: v=%d err=%v", v, err)
	}
	if loads != 1 {
		t.Fatalf("expected exactly 1 load, got %d", loads)
	}
}

func TestAside_Invalidate(t *testing.T) {
	c := newMemCache()
	ctx := context.Background()
	loads := 0
	load := func(context.Context) (string, error) {
		loads++

		return "v", nil
	}

	_, _ = cache.Aside(ctx, c, "k", time.Minute, load)
	cache.Invalidate(ctx, c, "k")
	_, _ = cache.Aside(ctx, c, "k", time.Minute, load) // miss again after invalidation

	if loads != 2 {
		t.Fatalf("expected 2 loads (reload after invalidate), got %d", loads)
	}
}

func TestAside_NilCacheAlwaysLoads(t *testing.T) {
	ctx := context.Background()
	loads := 0
	load := func(context.Context) (int, error) {
		loads++

		return 7, nil
	}

	for i := 0; i < 3; i++ {
		v, err := cache.Aside[int](ctx, nil, "k", time.Minute, load)
		if err != nil || v != 7 {
			t.Fatalf("v=%d err=%v", v, err)
		}
	}
	if loads != 3 {
		t.Fatalf("nil cache should load every time, got %d", loads)
	}
}

func TestAside_LoadErrorPropagates(t *testing.T) {
	c := newMemCache()
	sentinel := errors.New("boom")
	_, err := cache.Aside(context.Background(), c, "k", time.Minute, func(context.Context) (int, error) {
		return 0, sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected load error to propagate, got %v", err)
	}
}
