package cache_test

import (
	"testing"

	"github.com/aspirit/xsocial/pkg/cache"
)

func BenchmarkKey(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = cache.Key(cache.NSRateLimit, "login", "203.0.113.7", "device-abc")
	}
}
