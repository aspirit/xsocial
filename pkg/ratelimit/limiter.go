// Package ratelimit defines the rate limiting contract (interface, storage and policy constants).
// Implementations live in internal/ratelimit.
package ratelimit

import (
	"context"
	"time"
)

// Storage backend names for rate limiting. Used in config and when creating limiters.
const (
	StorageMemory = "memory"
	StorageCache  = "cache"
)

// Match type for policy path matching.
const (
	MatchTypeExact  = "exact"
	MatchTypePrefix = "prefix"
)

// Key mode for policy limit key (who is limited).
const (
	KeyModeIP                 = "ip"
	KeyModeIPDeviceFallbackIP = "ip_device_fallback_ip"
)

// Default window durations used in policy definitions.
const (
	Window1m  = "1m"
	Window10m = "10m"
)

// HTTP methods used in policy definitions.
const (
	MethodGet  = "GET"
	MethodPost = "POST"
)

// PolicyDef is the fixed definition for a rate-limit policy (method, pattern, match type, key mode, default window).
// Config supplies only name, requests, and optional window; the rest come from code (e.g. router).
type PolicyDef struct {
	Name          string
	Method        string
	Pattern       string
	MatchType     string // MatchTypeExact | MatchTypePrefix
	KeyMode       string // KeyModeIP | KeyModeIPDeviceFallbackIP
	DefaultWindow string // e.g. Window1m, Window10m
}

// Limiter is the interface for rate limit checks.
// Implementations: internal/ratelimit (memory, cache).
type Limiter interface {
	// Allow checks whether the key is allowed within the current window.
	// allowed: true = pass, false = reject.
	// retryAfter: seconds until window reset, only meaningful when allowed is false.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (allowed bool, retryAfter int, err error)
}
