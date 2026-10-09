package ratelimit

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aspirit/xsocial/internal/config"
	cachestorage "github.com/aspirit/xsocial/internal/storage/cache"
	"github.com/aspirit/xsocial/pkg/logger"
)

func TestCacheLimiter_Allow_Integration(t *testing.T) {
	addr := os.Getenv("CACHE_ADDRESS")
	if addr == "" {
		t.Skip("CACHE_ADDRESS not set, skipping cache limiter integration test")
	}
	redisCfg := config.RedisConfig{Address: addr, Password: os.Getenv("CACHE_PASSWORD")}
	cacheCfg := config.CacheConfig{Type: "redis"}
	logInst, err := logger.NewLogger(logger.LoggerConfig{Level: "info", OutputPath: "stdout", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}

	c, err := cachestorage.NewCache(cacheCfg, redisCfg, logInst)
	if err != nil {
		t.Skipf("cache not available: %v", err)
	}
	defer func() { _ = c.Close() }()

	lim, err := NewCacheLimiter(c, logInst)
	if err != nil {
		t.Fatalf("NewCacheLimiter: %v", err)
	}

	ctx := context.Background()
	key := "ratelimit_test_key_" + time.Now().Format("20060102150405")
	limit := 2
	window := 10 * time.Second

	for i := 0; i < limit; i++ {
		allowed, _, err := lim.Allow(ctx, key, limit, window)
		if err != nil {
			t.Fatalf("Allow: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}
	allowed, retryAfter, err := lim.Allow(ctx, key, limit, window)
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if allowed {
		t.Error("over limit should be rejected")
	}
	if retryAfter < 0 {
		t.Errorf("retryAfter should be >= 0, got %d", retryAfter)
	}
}
