package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/aspirit/xsocial/internal/config"
	"github.com/aspirit/xsocial/internal/ratelimit"
	httputil "github.com/aspirit/xsocial/pkg/http"
	"github.com/aspirit/xsocial/pkg/logger"
)

// countingLimiter enforces a per-key fixed limit by counting calls.
type countingLimiter struct {
	mu     sync.Mutex
	counts map[string]int
}

func newCountingLimiter() *countingLimiter { return &countingLimiter{counts: make(map[string]int)} }

func (l *countingLimiter) Allow(_ context.Context, key string, limit int, _ time.Duration) (bool, int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.counts[key]++
	if l.counts[key] > limit {
		return false, 1, nil
	}

	return true, 0, nil
}

func loginRateLimitConfig() *config.RateLimitConfig {
	return &config.RateLimitConfig{
		Enabled:               true,
		IPHardLimitMultiplier: 2, // IP ceiling = requests(2) * 2 = 4
		Global:                config.RateLimitRule{Requests: 1000, Window: "1m"},
		Policies: []config.RateLimitPolicy{{
			Name:      "login",
			Method:    http.MethodPost,
			Pattern:   "auth/login",
			MatchType: ratelimit.MatchTypeExact,
			KeyMode:   ratelimit.KeyModeIPDeviceFallbackIP,
			Requests:  2,
			Window:    "1m",
		}},
	}
}

func newLoginEngine(lim ratelimit.Limiter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(RateLimiter(lim, loginRateLimitConfig(), logger.NewNopLogger()))
	engine.POST("/v1/auth/login", func(c *gin.Context) { c.Status(http.StatusOK) })

	return engine
}

func doLogin(engine *gin.Engine, deviceID string) int {
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
	req.RemoteAddr = "192.0.2.10:4444"
	if deviceID != "" {
		req.Header.Set(httputil.HeaderDeviceID, deviceID)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	return w.Code
}

// TestRateLimiter_DeviceChurnHitsIPCeiling proves that cycling X-Device-ID from
// one IP cannot bypass the per-device limit: the second-order IP ceiling (4)
// blocks the 5th distinct-device request even though each device is under its
// own limit.
func TestRateLimiter_DeviceChurnHitsIPCeiling(t *testing.T) {
	engine := newLoginEngine(newCountingLimiter())

	for i := 1; i <= 4; i++ {
		code := doLogin(engine, "dev-"+strings.Repeat("x", i)) // distinct device each time
		assert.Equal(t, http.StatusOK, code, "request %d (distinct device) should pass", i)
	}
	// 5th distinct device: per-device bucket is fresh, but IP ceiling (4) is hit.
	assert.Equal(t, http.StatusTooManyRequests, doLogin(engine, "dev-yyyyy"))
}

// TestRateLimiter_PerDeviceLimitStillApplies proves the per-device limit blocks
// before the IP ceiling when one device repeats.
func TestRateLimiter_PerDeviceLimitStillApplies(t *testing.T) {
	engine := newLoginEngine(newCountingLimiter())

	assert.Equal(t, http.StatusOK, doLogin(engine, "dev-a"))
	assert.Equal(t, http.StatusOK, doLogin(engine, "dev-a"))
	// 3rd request from same device exceeds per-device limit of 2.
	assert.Equal(t, http.StatusTooManyRequests, doLogin(engine, "dev-a"))
}

func TestSanitizeDeviceID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"normal", "device-abc-123", "device-abc-123"},
		{"too long", strings.Repeat("a", maxDeviceIDLen+1), ""},
		{"control char", "dev\x00abc", ""},
		{"newline", "dev\nabc", ""},
		{"at max length", strings.Repeat("a", maxDeviceIDLen), strings.Repeat("a", maxDeviceIDLen)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sanitizeDeviceID(tt.in))
		})
	}
}
