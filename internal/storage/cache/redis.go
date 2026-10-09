// Package cache provides cache backend implementations (Redis, Memorystore, etc.).
package cache

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/aspirit/xsocial/internal/config"
	"github.com/aspirit/xsocial/pkg/cache"
	"github.com/aspirit/xsocial/pkg/logger"
)

// redisCache implements cache.Cache using go-redis.
// Works for both Redis and GCP Memorystore (same protocol).
type redisCache struct {
	client *redis.Client
	logger *logger.Logger
}

// NewRedisCache creates a Cache implementation using Redis/Memorystore.
func NewRedisCache(cfg config.RedisConfig, log *logger.Logger) (cache.Cache, error) {
	opts := &redis.Options{
		Addr:         cfg.Address,
		Password:     cfg.Password,
		DB:           cfg.Dbname,
		PoolSize:     cfg.PoolSize,
		MaxRetries:   cfg.MaxRetries,
		MinIdleConns: cfg.MinIdleConns,
	}

	if cfg.TLSEnabled {
		opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	if cfg.Timeout != "" {
		timeout, err := time.ParseDuration(cfg.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout format: %w", err)
		}
		opts.ReadTimeout = timeout
		opts.WriteTimeout = timeout
	}

	rdb := redis.NewClient(opts)

	if err := redisotel.InstrumentTracing(rdb); err != nil {
		log.Warn("Failed to enable cache tracing", zap.Error(err))
	}
	if err := redisotel.InstrumentMetrics(rdb); err != nil {
		log.Warn("Failed to enable cache metrics", zap.Error(err))
	}

	return &redisCache{
		client: rdb,
		logger: log.Named("cache-redis"),
	}, nil
}

// Get implements cache.Cache.
func (r *redisCache) Get(ctx context.Context, key string) (string, error) {
	result, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", cache.ErrNotFound
		}

		return "", fmt.Errorf("cache get: %w", err)
	}

	return result, nil
}

// Set implements cache.Cache.
func (r *redisCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	if err := r.client.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("cache set: %w", err)
	}

	return nil
}

// incrWithTTLScript atomically increments a counter and sets its TTL on the
// first increment, then returns {count, remainingPTTLms}. Running INCR + PEXPIRE
// in one script avoids the race a separate Get+Set would have.
var incrWithTTLScript = redis.NewScript(`
local c = redis.call('INCR', KEYS[1])
if c == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return {c, redis.call('PTTL', KEYS[1])}
`)

// IncrWithTTL implements cache.Cache.
func (r *redisCache) IncrWithTTL(ctx context.Context, key string, ttl time.Duration) (int64, time.Duration, error) {
	res, err := incrWithTTLScript.Run(ctx, r.client, []string{key}, ttl.Milliseconds()).Result()
	if err != nil {
		return 0, 0, fmt.Errorf("cache incr: %w", err)
	}
	vals, ok := res.([]any)
	if !ok || len(vals) != 2 {
		return 0, 0, fmt.Errorf("cache incr: %w", cache.ErrUnexpectedReply)
	}
	count, _ := vals[0].(int64)
	pttlMs, _ := vals[1].(int64)
	var remaining time.Duration
	if pttlMs > 0 {
		remaining = time.Duration(pttlMs) * time.Millisecond
	}

	return count, remaining, nil
}

// Del implements cache.Cache.
func (r *redisCache) Del(ctx context.Context, keys ...string) error {
	if err := r.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("cache del: %w", err)
	}

	return nil
}

// Ping implements cache.Cache.
func (r *redisCache) Ping(ctx context.Context) error {
	if err := r.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("cache ping: %w", err)
	}

	return nil
}

// Close implements cache.Cache.
func (r *redisCache) Close() error {
	if err := r.client.Close(); err != nil {
		return fmt.Errorf("cache close: %w", err)
	}

	return nil
}
