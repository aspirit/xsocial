package cache

import "strings"

// Namespace constants for Redis key partitioning.
// All services (api, worker) share one Redis instance;
// each usage domain MUST have its own namespace to avoid collisions.
const (
	NSPost      = "post" // Cached post documents (cache-aside).
	NSRateLimit = "rl"   // Rate limiting counters.
	// Future namespaces: add NSSession, NSLock, etc. here before first use.
)

// Key builds a namespaced cache key: "namespace:part1:part2:...".
// All Redis keys MUST be constructed through this function.
func Key(namespace string, parts ...string) string {
	if len(parts) == 0 {
		return namespace
	}

	return namespace + ":" + strings.Join(parts, ":")
}
