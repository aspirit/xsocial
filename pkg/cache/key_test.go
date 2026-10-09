package cache_test

import (
	"testing"

	"github.com/aspirit/xsocial/pkg/cache"
)

func TestKey(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		parts     []string
		want      string
	}{
		{"rate limit global", cache.NSRateLimit, []string{"global", "1.2.3.4"}, "rl:global:1.2.3.4"},
		{"rate limit policy with device", cache.NSRateLimit, []string{"login", "1.2.3.4", "dev-abc"}, "rl:login:1.2.3.4:dev-abc"},
		{"post", cache.NSPost, []string{"42"}, "post:42"},
		{"single part", cache.NSRateLimit, []string{"global"}, "rl:global"},
		{"namespace only", cache.NSRateLimit, nil, "rl"},
		{"empty parts slice", "ns", []string{}, "ns"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cache.Key(tt.namespace, tt.parts...)
			if got != tt.want {
				t.Errorf("Key(%q, %v) = %q, want %q", tt.namespace, tt.parts, got, tt.want)
			}
		})
	}
}
