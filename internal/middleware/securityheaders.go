package middleware

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

// Security header names.
const (
	HeaderXContentTypeOptions     = "X-Content-Type-Options"
	HeaderXFrameOptions           = "X-Frame-Options"
	HeaderReferrerPolicy          = "Referrer-Policy"
	HeaderStrictTransportSecurity = "Strict-Transport-Security"
	HeaderContentSecurityPolicy   = "Content-Security-Policy"
)

// DefaultAPIContentSecurityPolicy is a minimal CSP for a JSON API: it serves no
// active content and must never be framed. Page-level CSP is the frontend's
// responsibility (coordinated separately).
const DefaultAPIContentSecurityPolicy = "default-src 'none'; frame-ancestors 'none'"

const defaultHSTSMaxAgeSeconds = 31536000 // 1 year

// SecurityHeadersConfig configures the security headers middleware.
type SecurityHeadersConfig struct {
	// EnableHSTS adds Strict-Transport-Security. Enable only when traffic is
	// served over HTTPS (e.g. release mode behind a TLS-terminating gateway);
	// HSTS over plain HTTP is ignored by browsers and confuses local dev.
	EnableHSTS bool
	// HSTSMaxAgeSeconds is the HSTS max-age; defaults to one year when <= 0.
	HSTSMaxAgeSeconds int
	// ContentSecurityPolicy overrides the default API CSP when non-empty.
	ContentSecurityPolicy string
}

// SecurityHeaders returns middleware that sets baseline security response
// headers on every response (including errors and preflight) so the API is
// hardened against MIME sniffing, clickjacking and referrer leakage.
func SecurityHeaders(cfg SecurityHeadersConfig) gin.HandlerFunc {
	csp := cfg.ContentSecurityPolicy
	if csp == "" {
		csp = DefaultAPIContentSecurityPolicy
	}
	maxAge := cfg.HSTSMaxAgeSeconds
	if maxAge <= 0 {
		maxAge = defaultHSTSMaxAgeSeconds
	}
	hstsValue := fmt.Sprintf("max-age=%d; includeSubDomains", maxAge)

	return func(c *gin.Context) {
		c.Header(HeaderXContentTypeOptions, "nosniff")
		c.Header(HeaderXFrameOptions, "DENY")
		c.Header(HeaderReferrerPolicy, "no-referrer")
		c.Header(HeaderContentSecurityPolicy, csp)
		if cfg.EnableHSTS {
			c.Header(HeaderStrictTransportSecurity, hstsValue)
		}
		c.Next()
	}
}
