// Package cache provides a generic cache interface for key-value storage.
// Supports Redis (local), Memorystore (GCP), and future backends (e.g. Memcached).
package cache

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when a key does not exist.
var ErrNotFound = errors.New("cache: key not found")

// ErrUnexpectedReply is returned when a backend command returns a reply of an
// unexpected shape (e.g. a Lua script result that is not the expected tuple).
var ErrUnexpectedReply = errors.New("cache: unexpected reply shape")

// Cache defines the interface for cache operations.
// Implementations: Redis, Memorystore (Redis-compatible), Memcached (future).
type Cache interface {
	// Get returns the value for key. Returns ErrNotFound if key does not exist.
	Get(ctx context.Context, key string) (string, error)

	// Set stores key with value and optional TTL. Use 0 for no expiration.
	Set(ctx context.Context, key string, value any, ttl time.Duration) error

	// IncrWithTTL atomically increments the integer counter at key, setting ttl
	// only on the first increment (when the key is created). It returns the new
	// counter value and the key's remaining TTL. Use it for atomic fixed-window
	// rate limiting, where a separate Get+Set would race under concurrency.
	IncrWithTTL(ctx context.Context, key string, ttl time.Duration) (count int64, remaining time.Duration, err error)

	// Del deletes one or more keys.
	Del(ctx context.Context, keys ...string) error

	// Ping checks if the cache is reachable.
	Ping(ctx context.Context) error

	// Close closes the cache client.
	Close() error
}

// ProviderType represents the cache backend type.
type ProviderType string

const (
	ProviderRedis ProviderType = "redis"
	// ProviderMemorystore uses Redis protocol (GCP managed). Same impl as redis, TLS optional.
	ProviderMemorystore ProviderType = "memorystore"
)

// IsValid checks if the provider type is valid.
func (p ProviderType) IsValid() bool {
	switch p {
	case ProviderRedis, ProviderMemorystore:
		return true
	default:
		return false
	}
}
