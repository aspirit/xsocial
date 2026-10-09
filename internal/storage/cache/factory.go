// Package cache provides cache backend implementations and factory.
package cache

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aspirit/xsocial/internal/config"
	"github.com/aspirit/xsocial/pkg/cache"
	"github.com/aspirit/xsocial/pkg/logger"
)

// ErrUnsupportedCacheType is returned when storage.cache.type is not redis or memorystore.
var ErrUnsupportedCacheType = errors.New("unsupported cache type")

// NewCache creates a Cache based on storage.cache and storage.redis configuration.
// Type: redis (local) or memorystore (GCP) - both use Redis protocol.
func NewCache(cacheCfg config.CacheConfig, redisCfg config.RedisConfig, log *logger.Logger) (cache.Cache, error) {
	t := strings.ToLower(strings.TrimSpace(cacheCfg.Type))
	if t == "" {
		t = string(cache.ProviderRedis)
	}

	switch cache.ProviderType(t) {
	case cache.ProviderRedis, cache.ProviderMemorystore:
		return NewRedisCache(redisCfg, log)
	default:
		return nil, fmt.Errorf("%w: %q (use redis or memorystore)", ErrUnsupportedCacheType, t)
	}
}
