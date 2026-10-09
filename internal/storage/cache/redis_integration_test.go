// Integration tests for cache Redis implementation.
// Requires Redis running (e.g. docker-compose up redis). Skips if unavailable.
package cache

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aspirit/xsocial/internal/config"
	"github.com/aspirit/xsocial/pkg/cache"
	"github.com/aspirit/xsocial/pkg/logger"
)

func TestRedisCache_Integration(t *testing.T) {
	addr := os.Getenv("CACHE_ADDRESS")
	if addr == "" {
		addr = "localhost:6379"
	}
	password := os.Getenv("CACHE_PASSWORD")
	if password == "" {
		password = "redis-password" // matches dev/docker/docker-compose
	}

	log, err := logger.NewLogger(logger.LoggerConfig{
		Level:       "info",
		Development: true,
		ServiceID:   "cache-integration-test",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	cfg := config.RedisConfig{
		Address:      addr,
		Password:     password,
		Dbname:       0,
		PoolSize:     5,
		MaxRetries:   2,
		MinIdleConns: 1,
		TLSEnabled:   false,
	}

	c, err := NewRedisCache(cfg, log)
	if err != nil {
		t.Skipf("Skipping: Redis not available at %s: %v", addr, err)
	}
	defer func() { _ = c.Close() }()

	ctx := context.Background()
	key := "cache:integration:test:" + time.Now().Format("20060102150405")

	// Ping: the client connects lazily, so availability is only known here.
	// CI sets CI_STRICT_INTEGRATION=1 so a missing Redis fails instead of skipping.
	if err := c.Ping(ctx); err != nil {
		if os.Getenv("CI_STRICT_INTEGRATION") == "1" {
			t.Fatalf("Ping failed: %v", err)
		}
		t.Skipf("Skipping: Redis not reachable at %s: %v", addr, err)
	}

	// Get missing key -> ErrNotFound
	_, err = c.Get(ctx, key)
	if err != cache.ErrNotFound {
		t.Errorf("Get missing key: expected ErrNotFound, got %v", err)
	}

	// Set
	if err := c.Set(ctx, key, "hello", 30*time.Second); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get
	val, err := c.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "hello" {
		t.Errorf("Get: expected 'hello', got %q", val)
	}

	// Del
	if err := c.Del(ctx, key); err != nil {
		t.Fatalf("Del failed: %v", err)
	}

	// Get after Del -> ErrNotFound
	_, err = c.Get(ctx, key)
	if err != cache.ErrNotFound {
		t.Errorf("Get after Del: expected ErrNotFound, got %v", err)
	}

	// IncrWithTTL: first increment sets TTL and returns 1; subsequent ones grow.
	incrKey := key + ":incr"
	defer func() { _ = c.Del(context.Background(), incrKey) }()
	count, remaining, err := c.IncrWithTTL(ctx, incrKey, 30*time.Second)
	if err != nil {
		t.Fatalf("IncrWithTTL failed: %v", err)
	}
	if count != 1 {
		t.Errorf("IncrWithTTL first call: expected count 1, got %d", count)
	}
	if remaining <= 0 || remaining > 30*time.Second {
		t.Errorf("IncrWithTTL first call: expected TTL in (0,30s], got %v", remaining)
	}
	count, _, err = c.IncrWithTTL(ctx, incrKey, 30*time.Second)
	if err != nil {
		t.Fatalf("IncrWithTTL second call failed: %v", err)
	}
	if count != 2 {
		t.Errorf("IncrWithTTL second call: expected count 2, got %d", count)
	}
}
