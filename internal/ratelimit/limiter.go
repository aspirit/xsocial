// Package ratelimit provides rate limiting implementations for the API service.
// Contract (Limiter interface, Storage constants) is in pkg/ratelimit; this package implements it.
package ratelimit

import (
	pkgratelimit "github.com/aspirit/xsocial/pkg/ratelimit"
)

// Re-export from pkg/ratelimit so callers can use this package for both the interface and implementations.
var (
	_ pkgratelimit.Limiter = (*MemoryLimiter)(nil)
	_ pkgratelimit.Limiter = (*CacheLimiter)(nil)
)

// Limiter, storage and policy constants/types are defined in pkg/ratelimit; re-export for convenience.
type Limiter = pkgratelimit.Limiter
type PolicyDef = pkgratelimit.PolicyDef

const (
	StorageMemory = pkgratelimit.StorageMemory
	StorageCache  = pkgratelimit.StorageCache

	MatchTypeExact  = pkgratelimit.MatchTypeExact
	MatchTypePrefix = pkgratelimit.MatchTypePrefix

	KeyModeIP                 = pkgratelimit.KeyModeIP
	KeyModeIPDeviceFallbackIP = pkgratelimit.KeyModeIPDeviceFallbackIP

	Window1m  = pkgratelimit.Window1m
	Window10m = pkgratelimit.Window10m

	MethodGet  = pkgratelimit.MethodGet
	MethodPost = pkgratelimit.MethodPost
)
