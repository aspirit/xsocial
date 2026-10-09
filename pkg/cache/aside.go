package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aspirit/xsocial/pkg/otelutil"
)

// Aside implements the cache-aside pattern for a typed value. On a cache hit it
// returns the decoded value (and records cache_hit_total); on a miss — or any
// cache/decode error — it records cache_miss_total, calls load, caches the
// JSON-encoded result with ttl, and returns it.
//
// Cache failures never fail the call (fail-open): load is the source of truth.
// Pass a nil Cache to bypass caching entirely (useful when cache is optional).
//
// Read paths adopt this by holding a Cache and calling Aside with a namespaced
// key (see pkg/cache.Key); writes must call Invalidate for the same key.
func Aside[T any](ctx context.Context, c Cache, key string, ttl time.Duration, load func(context.Context) (T, error)) (T, error) {
	if c != nil {
		if raw, err := c.Get(ctx, key); err == nil {
			var v T
			if json.Unmarshal([]byte(raw), &v) == nil {
				recordCacheCounter(ctx, "cache_hit_total")

				return v, nil
			}
		}
		recordCacheCounter(ctx, "cache_miss_total")
	}

	v, err := load(ctx)
	if err != nil {
		var zero T

		return zero, fmt.Errorf("cache-aside load %q: %w", key, err)
	}

	if c != nil {
		if b, mErr := json.Marshal(v); mErr == nil {
			_ = c.Set(ctx, key, string(b), ttl)
		}
	}

	return v, nil
}

// Invalidate deletes cached keys; call it after a write so the next read
// reloads. A nil cache or empty key list is a no-op.
func Invalidate(ctx context.Context, c Cache, keys ...string) {
	if c == nil || len(keys) == 0 {
		return
	}
	_ = c.Del(ctx, keys...)
}

// recordCacheCounter increments a counter on the global telemetry provider if
// one is initialized (no-op otherwise), so hit-rate is observable on /metrics.
func recordCacheCounter(ctx context.Context, name string) {
	if t := otelutil.Default(); t != nil {
		t.RecordCounter(ctx, name, 1)
	}
}
