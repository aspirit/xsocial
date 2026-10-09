package middleware

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/aspirit/xsocial/internal/config"
	internalerr "github.com/aspirit/xsocial/internal/errors"
	"github.com/aspirit/xsocial/internal/http/resp"
	"github.com/aspirit/xsocial/internal/ratelimit"
	"github.com/aspirit/xsocial/pkg/cache"
	httputil "github.com/aspirit/xsocial/pkg/http"
	"github.com/aspirit/xsocial/pkg/logger"
	"github.com/aspirit/xsocial/pkg/otelutil"
)

// defaultIPHardLimitMultiplier caps a single IP at policy.Requests * N for
// device-scoped policies, so churning X-Device-ID cannot bypass the limit.
const defaultIPHardLimitMultiplier = 5

// maxDeviceIDLen bounds X-Device-ID. Anything longer (or containing control
// characters) is treated as absent and the request is keyed by IP only — this
// is non-breaking (legit IDs are short) and prevents cache-key abuse.
const maxDeviceIDLen = 128

// RateLimiter returns a Gin middleware that enforces global and policy-based rate limits.
// Mount it on the v1 group so /status and /swagger are not limited.
// When cfg is nil or cfg.Enabled is false, the middleware passes all requests.
func RateLimiter(limiter ratelimit.Limiter, cfg *config.RateLimitConfig, log *logger.Logger) gin.HandlerFunc {
	if cfg == nil || !cfg.Enabled {
		return func(c *gin.Context) { c.Next() }
	}

	globalWindow, _ := time.ParseDuration(cfg.Global.Window)
	ipMultiplier := cfg.IPHardLimitMultiplier
	if ipMultiplier <= 0 {
		ipMultiplier = defaultIPHardLimitMultiplier
	}
	parsedPolicies := make([]policyWithWindow, 0, len(cfg.Policies))
	for i := range cfg.Policies {
		w, err := time.ParseDuration(cfg.Policies[i].Window)
		if err != nil {
			continue
		}
		parsedPolicies = append(parsedPolicies, policyWithWindow{
			RateLimitPolicy: &cfg.Policies[i],
			window:          w,
		})
	}

	return func(c *gin.Context) {
		req := requestInfo{
			ip:             c.ClientIP(),
			normalizedPath: normalizePath(c.FullPath()),
			method:         c.Request.Method,
			deviceID:       sanitizeDeviceID(c.GetHeader(httputil.HeaderDeviceID)),
		}
		if applyGlobalLimit(c, limiter, cfg, globalWindow, req, log) {
			return
		}
		policy := matchPolicy(parsedPolicies, req.method, req.normalizedPath)
		if policy == nil {
			c.Next()

			return
		}
		if applyPolicyLimit(c, limiter, policy, req, ipMultiplier, log) {
			return
		}
		c.Next()
	}
}

// sanitizeDeviceID returns the device ID if it is a plausible identifier,
// otherwise "" so the caller falls back to IP-only keying. Strict format
// enforcement (rejecting the request) requires frontend coordination and is
// intentionally deferred.
func sanitizeDeviceID(deviceID string) string {
	if deviceID == "" || len(deviceID) > maxDeviceIDLen {
		return ""
	}
	for _, r := range deviceID {
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}

	return deviceID
}

type requestInfo struct {
	ip             string
	normalizedPath string
	method         string
	deviceID       string
}

func applyGlobalLimit(c *gin.Context, limiter ratelimit.Limiter, cfg *config.RateLimitConfig, globalWindow time.Duration, req requestInfo, log *logger.Logger) bool {
	allowed, retryAfter, err := limiter.Allow(c.Request.Context(), cache.Key(cache.NSRateLimit, "global", req.ip), cfg.Global.Requests, globalWindow)
	if err != nil {
		log.Error("rate limiter global check failed", zap.Error(err), zap.String("ip", req.ip), zap.String("path", req.normalizedPath))
		c.Next()

		return true
	}
	if !allowed {
		respondRateLimited(c, retryAfter, log, rateLimitLogFields{
			Policy: "global", IP: req.ip, Path: req.normalizedPath, Method: req.method,
			Limit: cfg.Global.Requests, Window: cfg.Global.Window, RetryAfter: retryAfter,
		})

		return true
	}

	return false
}

func applyPolicyLimit(c *gin.Context, limiter ratelimit.Limiter, policy *policyWithWindow, req requestInfo, ipMultiplier int, log *logger.Logger) bool {
	// Primary limit: device-scoped when a device ID is present, else IP-scoped.
	key := buildPolicyKey(policy.RateLimitPolicy, req.ip, req.deviceID)
	if blocked := enforceLimit(c, limiter, key, policy.Requests, policy.window, policy.Name, req, log); blocked {
		return true
	}

	// Second-order IP ceiling: for device-fallback policies keyed by device,
	// also cap the IP so cycling X-Device-ID cannot bypass the per-device limit.
	if policy.KeyMode == ratelimit.KeyModeIPDeviceFallbackIP && req.deviceID != "" {
		ipKey := cache.Key(cache.NSRateLimit, policy.Name, "ip", req.ip)

		return enforceLimit(c, limiter, ipKey, policy.Requests*ipMultiplier, policy.window, policy.Name+":ip", req, log)
	}

	return false
}

// enforceLimit runs one limiter check and, on rejection, responds 429. It
// returns true when the request was handled (blocked or errored, fail-open).
func enforceLimit(c *gin.Context, limiter ratelimit.Limiter, key string, limit int, window time.Duration, policyName string, req requestInfo, log *logger.Logger) bool {
	allowed, retryAfter, err := limiter.Allow(c.Request.Context(), key, limit, window)
	if err != nil {
		log.Error("rate limiter policy check failed", zap.Error(err), zap.String("policy", policyName), zap.String("ip", req.ip))
		c.Next()

		return true
	}
	if !allowed {
		respondRateLimited(c, retryAfter, log, rateLimitLogFields{
			Policy: policyName, IP: req.ip, Path: req.normalizedPath, Method: req.method,
			Limit: limit, Window: window.String(), RetryAfter: retryAfter,
		})

		return true
	}

	return false
}

type policyWithWindow struct {
	*config.RateLimitPolicy
	window time.Duration
}

func normalizePath(fullPath string) string {
	s := strings.TrimPrefix(strings.Trim(fullPath, "/"), "v1/")

	return strings.TrimPrefix(s, "/")
}

func matchPolicy(policies []policyWithWindow, method, normalizedPath string) *policyWithWindow {
	for i := range policies {
		p := &policies[i]
		if p.Method != method {
			continue
		}
		switch p.MatchType {
		case ratelimit.MatchTypeExact:
			if p.Pattern == normalizedPath {
				return p
			}
		case ratelimit.MatchTypePrefix:
			prefix := strings.TrimSuffix(p.Pattern, "/*")
			if prefix != p.Pattern {
				prefix += "/"
			}
			// Match exact path or path under prefix (segment boundary: "auth/login" or "auth/login/..." only).
			if normalizedPath == prefix || strings.HasPrefix(normalizedPath, prefix+"/") {
				return p
			}
		}
	}

	return nil
}

func buildPolicyKey(p *config.RateLimitPolicy, ip, deviceID string) string {
	if p.KeyMode == ratelimit.KeyModeIPDeviceFallbackIP && deviceID != "" {
		return cache.Key(cache.NSRateLimit, p.Name, ip, deviceID)
	}

	return cache.Key(cache.NSRateLimit, p.Name, ip)
}

type rateLimitLogFields struct {
	Policy     string `json:"policy"`
	IP         string `json:"ip"`
	Path       string `json:"path"`
	Method     string `json:"method"`
	Limit      int    `json:"limit"`
	Window     string `json:"window"`
	RetryAfter int    `json:"retry_after"`
}

func respondRateLimited(c *gin.Context, retryAfter int, log *logger.Logger, fields rateLimitLogFields) {
	c.Header("Retry-After", strconv.Itoa(retryAfter))
	log.Info("rate limit exceeded",
		zap.String("event", "rate_limit_exceeded"),
		zap.Any("details", fields))
	otelutil.Count(c.Request.Context(), "ratelimit_exceeded_total", 1)
	resp.Fail(c, internalerr.ErrRequestLimitExceeded)
	c.Abort()
}
