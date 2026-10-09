package middleware

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/aspirit/xsocial/pkg/server"
	"github.com/gin-gonic/gin"
)

// CORS configuration errors.
var (
	ErrWildcardWithCredentials     = errors.New("cannot use wildcard origin (*) with allow_credentials=true")
	ErrEmptyOriginsWithCredentials = errors.New("allow_origins cannot be empty when allow_credentials=true")
)

// CORS header constants.
const (
	HeaderAccessControlAllowOrigin      = "Access-Control-Allow-Origin"
	HeaderAccessControlAllowMethods     = "Access-Control-Allow-Methods"
	HeaderAccessControlAllowHeaders     = "Access-Control-Allow-Headers"
	HeaderAccessControlExposeHeaders    = "Access-Control-Expose-Headers"
	HeaderAccessControlAllowCredentials = "Access-Control-Allow-Credentials"
	HeaderAccessControlMaxAge           = "Access-Control-Max-Age"
)

// CORS creates a configurable CORS middleware.
func CORS(config *server.CORSConfig) gin.HandlerFunc {
	// If CORS is disabled, return a no-op middleware
	if config == nil || !config.Enabled {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	// Validate CORS configuration
	if err := validateCORSConfig(config); err != nil {
		// Log error and return a restrictive middleware
		log.Printf("CORS configuration error: %v, using restrictive mode", err)

		return restrictiveCORS()
	}

	// Pre-compute header values for performance
	allowMethods := strings.Join(server.GetCORSMethods(), ", ")
	allowHeaders := strings.Join(server.GetCORSHeaders(), ", ")
	exposeHeaders := strings.Join(server.GetCORSExposeHeaders(), ", ")
	maxAge := fmt.Sprintf("%d", server.GetCORSMaxAge())

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		method := c.Request.Method

		// Set CORS headers for all requests
		setCORSHeaders(c, origin, config, allowMethods, allowHeaders, exposeHeaders, maxAge)

		// Handle preflight requests
		if method == "OPTIONS" {
			c.Status(http.StatusNoContent)
			c.Abort()

			return
		}

		c.Next()
	}
}

// setCORSHeaders sets CORS headers for a request.
func setCORSHeaders(c *gin.Context, origin string, config *server.CORSConfig,
	allowMethods, allowHeaders, exposeHeaders, maxAge string,
) {
	if origin == "" {
		return
	}

	// Set origin header
	setOriginHeader(c, origin, config)

	// Set other CORS headers
	c.Header(HeaderAccessControlAllowMethods, allowMethods)
	c.Header(HeaderAccessControlAllowHeaders, allowHeaders)
	c.Header(HeaderAccessControlExposeHeaders, exposeHeaders)
	c.Header(HeaderAccessControlMaxAge, maxAge)

	// Handle credentials
	if config.AllowCredentials {
		c.Header(HeaderAccessControlAllowCredentials, "true")
	}
}

// setOriginHeader sets the Access-Control-Allow-Origin header.
func setOriginHeader(c *gin.Context, origin string, config *server.CORSConfig) {
	if isOriginAllowed(origin, config.AllowOrigins, config.AllowWildcardSubdomains) {
		c.Header(HeaderAccessControlAllowOrigin, origin)
	} else if contains(config.AllowOrigins, "*") && !config.AllowCredentials {
		// Only allow wildcard if credentials are disabled
		c.Header(HeaderAccessControlAllowOrigin, "*")
	}
}

// isOriginAllowed checks if the given origin is allowed based on the configuration.
func isOriginAllowed(origin string, allowedOrigins []string, allowWildcardSubdomains bool) bool {
	for _, allowed := range allowedOrigins {
		if allowed == "*" {
			return true
		}
		if allowed == origin {
			return true
		}
		if allowWildcardSubdomains && isWildcardSubdomain(origin, allowed) {
			return true
		}
	}

	return false
}

// isWildcardSubdomain checks if origin matches a wildcard subdomain pattern.
func isWildcardSubdomain(origin, pattern string) bool {
	if !strings.HasPrefix(pattern, "*.") {
		return false
	}

	domain := pattern[2:] // Remove "*."
	// More secure matching: must end with "."+domain or be exactly the domain
	return strings.HasSuffix(origin, "."+domain) || origin == domain
}

// contains checks if a string slice contains a specific string.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}

	return false
}

// validateCORSConfig validates CORS configuration for security issues.
func validateCORSConfig(config *server.CORSConfig) error {
	// Check for dangerous combination: wildcard origin with credentials
	if contains(config.AllowOrigins, "*") && config.AllowCredentials {
		return ErrWildcardWithCredentials //nolint:wrapcheck // Static error, no need to wrap
	}

	// Check for empty origins when credentials are enabled
	if config.AllowCredentials && len(config.AllowOrigins) == 0 {
		return ErrEmptyOriginsWithCredentials //nolint:wrapcheck // Static error, no need to wrap
	}

	return nil
}

// restrictiveCORS returns a restrictive CORS middleware for error cases.
func restrictiveCORS() gin.HandlerFunc {
	// Use minimal safe configuration
	allowMethods := "GET, POST, OPTIONS"
	allowHeaders := "Content-Type, Authorization"
	maxAge := "3600"

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		method := c.Request.Method

		// Only allow localhost origins in restrictive mode
		if origin != "" && isLocalhost(origin) {
			c.Header(HeaderAccessControlAllowOrigin, origin)
			c.Header(HeaderAccessControlAllowMethods, allowMethods)
			c.Header(HeaderAccessControlAllowHeaders, allowHeaders)
			c.Header(HeaderAccessControlMaxAge, maxAge)
		}

		if method == "OPTIONS" {
			c.Status(http.StatusNoContent)
			c.Abort()

			return
		}

		c.Next()
	}
}

// isLocalhost checks if the origin is a localhost address.
func isLocalhost(origin string) bool {
	return strings.HasPrefix(origin, "http://localhost:") ||
		strings.HasPrefix(origin, "http://127.0.0.1:")
}
