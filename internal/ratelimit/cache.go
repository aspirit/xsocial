package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/aspirit/xsocial/pkg/cache"
	"github.com/aspirit/xsocial/pkg/logger"
)

// ErrCacheRequired is returned when cache storage is selected but cache is nil.
var ErrCacheRequired = errors.New("cache required for ratelimit when storage is cache")

// CacheLimiter implements Limiter using the shared cache (Redis/Memorystore/etc.).
// Fixed-window: an atomic INCR sets the TTL on the first request in the window
// (see cache.IncrWithTTL), so concurrent requests count correctly. On cache
// errors it fails open (allows request, returns error). Does not own the cache.
type CacheLimiter struct {
	cache cache.Cache
	log   *logger.Logger
}

// NewCacheLimiter creates a cache-backed rate limiter. The cache is shared and must not be closed by the limiter.
func NewCacheLimiter(c cache.Cache, log *logger.Logger) (*CacheLimiter, error) {
	if c == nil {
		return nil, fmt.Errorf("%w", ErrCacheRequired)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Ping(ctx); err != nil {
		return nil, fmt.Errorf("cache ping: %w", err)
	}

	return &CacheLimiter{cache: c, log: log.Named("ratelimit-cache")}, nil
}

// Allow implements Limiter with an atomic fixed-window counter (INCR + PEXPIRE
// in one round-trip). On cache errors it fails open (allows the request and
// surfaces the error) so a cache outage does not lock users out.
func (c *CacheLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (allowed bool, retryAfter int, err error) {
	count, remaining, err := c.cache.IncrWithTTL(ctx, key, window)
	if err != nil {
		c.log.Error("cache rate limit incr failed", zap.Error(err), zap.String("key", key))

		return true, 0, fmt.Errorf("cache incr: %w", err)
	}
	if count > int64(limit) {
		retryAfter = int(remaining.Seconds())
		if retryAfter <= 0 {
			retryAfter = int(window.Seconds())
		}

		return false, retryAfter, nil
	}

	return true, 0, nil
}
