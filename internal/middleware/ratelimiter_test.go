package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aspirit/xsocial/internal/config"
	"github.com/aspirit/xsocial/internal/ratelimit"
	"github.com/aspirit/xsocial/pkg/logger"
)

// mockLimiter implements ratelimit.Limiter for tests. AllowFunc is called for each Allow().
type mockLimiter struct {
	AllowFunc func(ctx context.Context, key string, limit int, window time.Duration) (allowed bool, retryAfter int, err error)
}

func (m *mockLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, int, error) {
	if m.AllowFunc != nil {
		return m.AllowFunc(ctx, key, limit, window)
	}

	return true, 0, nil
}

func TestRateLimiter_Disabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := logger.NewNopLogger()
	lim := &mockLimiter{}

	// cfg nil -> pass through
	h := RateLimiter(lim, nil, log)
	engine := gin.New()
	engine.Use(h)
	engine.GET("/v1/any", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v1/any", nil)
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRateLimiter_NotEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := logger.NewNopLogger()
	lim := &mockLimiter{}
	cfg := &config.RateLimitConfig{Enabled: false}

	h := RateLimiter(lim, cfg, log)
	engine := gin.New()
	engine.Use(h)
	engine.GET("/v1/any", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v1/any", nil)
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRateLimiter_GlobalOverLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := logger.NewNopLogger()
	called := false
	lim := &mockLimiter{
		AllowFunc: func(_ context.Context, key string, _ int, _ time.Duration) (bool, int, error) {
			if key == "rl:global:127.0.0.1" {
				called = true

				return false, 42, nil
			}

			return true, 0, nil
		},
	}
	cfg := &config.RateLimitConfig{
		Enabled:  true,
		Global:   config.RateLimitRule{Requests: 120, Window: "1m"},
		Policies: []config.RateLimitPolicy{},
	}

	h := RateLimiter(lim, cfg, log)
	engine := gin.New()
	engine.Use(h)
	engine.GET("/v1/status", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v1/status", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	engine.ServeHTTP(w, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "42", w.Header().Get("Retry-After"))
}

func TestRateLimiter_PolicyOverLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := logger.NewNopLogger()
	policyCalled := false
	lim := &mockLimiter{
		AllowFunc: func(_ context.Context, key string, _ int, _ time.Duration) (bool, int, error) {
			if key == "rl:global:127.0.0.1" {
				return true, 0, nil
			}
			if len(key) > 8 && key[:8] == "rl:login" {
				policyCalled = true

				return false, 10, nil
			}

			return true, 0, nil
		},
	}
	cfg := &config.RateLimitConfig{
		Enabled: true,
		Global:  config.RateLimitRule{Requests: 120, Window: "1m"},
		Policies: []config.RateLimitPolicy{
			{
				Name: "login", Method: "POST", Pattern: "auth/login",
				MatchType: ratelimit.MatchTypePrefix, KeyMode: ratelimit.KeyModeIP,
				Requests: 5, Window: "1m",
			},
		},
	}

	h := RateLimiter(lim, cfg, log)
	engine := gin.New()
	engine.Use(h)
	engine.POST("/v1/auth/login/username", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/v1/auth/login/username", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	engine.ServeHTTP(w, req)

	assert.True(t, policyCalled)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "10", w.Header().Get("Retry-After"))
}

func TestRateLimiter_FailOpen(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := logger.NewNopLogger()
	lim := &mockLimiter{
		AllowFunc: func(_ context.Context, _ string, _ int, _ time.Duration) (bool, int, error) {
			return false, 0, assert.AnError
		},
	}
	cfg := &config.RateLimitConfig{
		Enabled:  true,
		Global:   config.RateLimitRule{Requests: 120, Window: "1m"},
		Policies: []config.RateLimitPolicy{},
	}

	h := RateLimiter(lim, cfg, log)
	engine := gin.New()
	engine.Use(h)
	engine.GET("/v1/status", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v1/status", nil)
	engine.ServeHTTP(w, req)

	// Fail-open: error from limiter -> allow request
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRateLimiter_PrefixBoundary_NoMatch(t *testing.T) {
	// Path "auth/loginx" must NOT match policy with prefix "auth/login" (segment boundary).
	gin.SetMode(gin.TestMode)
	log := logger.NewNopLogger()
	policyKeyCalled := false
	lim := &mockLimiter{
		AllowFunc: func(_ context.Context, key string, _ int, _ time.Duration) (bool, int, error) {
			if key == "rl:global:127.0.0.1" {
				return true, 0, nil
			}
			if len(key) > 8 && key[:8] == "rl:login" {
				policyKeyCalled = true

				return false, 5, nil
			}

			return true, 0, nil
		},
	}
	cfg := &config.RateLimitConfig{
		Enabled: true,
		Global:  config.RateLimitRule{Requests: 120, Window: "1m"},
		Policies: []config.RateLimitPolicy{
			{
				Name: "login", Method: "GET", Pattern: "auth/login",
				MatchType: ratelimit.MatchTypePrefix, KeyMode: ratelimit.KeyModeIP,
				Requests: 5, Window: "1m",
			},
		},
	}

	h := RateLimiter(lim, cfg, log)
	engine := gin.New()
	engine.Use(h)
	engine.GET("/v1/auth/loginx", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v1/auth/loginx", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	engine.ServeHTTP(w, req)

	// No policy matched (auth/loginx != auth/login and no auth/login/ prefix)
	assert.False(t, policyKeyCalled)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRateLimiter_PrefixBoundary_Match(t *testing.T) {
	// Path "auth/login/username" must match policy with prefix "auth/login".
	gin.SetMode(gin.TestMode)
	log := logger.NewNopLogger()
	policyKeyCalled := false
	lim := &mockLimiter{
		AllowFunc: func(_ context.Context, key string, _ int, _ time.Duration) (bool, int, error) {
			if key == "rl:global:127.0.0.1" {
				return true, 0, nil
			}
			if len(key) > 8 && key[:8] == "rl:login" {
				policyKeyCalled = true

				return true, 0, nil // allow so we get 200 and confirm policy was applied
			}

			return true, 0, nil
		},
	}
	cfg := &config.RateLimitConfig{
		Enabled: true,
		Global:  config.RateLimitRule{Requests: 120, Window: "1m"},
		Policies: []config.RateLimitPolicy{
			{
				Name: "login", Method: "POST", Pattern: "auth/login",
				MatchType: ratelimit.MatchTypePrefix, KeyMode: ratelimit.KeyModeIP,
				Requests: 5, Window: "1m",
			},
		},
	}

	h := RateLimiter(lim, cfg, log)
	engine := gin.New()
	engine.Use(h)
	engine.POST("/v1/auth/login/username", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/v1/auth/login/username", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	engine.ServeHTTP(w, req)

	require.True(t, policyKeyCalled, "policy key should have been checked")
	assert.Equal(t, http.StatusOK, w.Code)
}
