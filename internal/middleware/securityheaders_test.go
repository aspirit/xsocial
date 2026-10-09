package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func newSecurityHeadersEngine(cfg SecurityHeadersConfig) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(SecurityHeaders(cfg))
	engine.GET("/x", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	return engine
}

func TestSecurityHeaders_BaselineSet(t *testing.T) {
	engine := newSecurityHeadersEngine(SecurityHeadersConfig{})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "nosniff", w.Header().Get(HeaderXContentTypeOptions))
	assert.Equal(t, "DENY", w.Header().Get(HeaderXFrameOptions))
	assert.Equal(t, "no-referrer", w.Header().Get(HeaderReferrerPolicy))
	assert.Equal(t, DefaultAPIContentSecurityPolicy, w.Header().Get(HeaderContentSecurityPolicy))
	// HSTS off by default (non-HTTPS / dev).
	assert.Empty(t, w.Header().Get(HeaderStrictTransportSecurity))
}

func TestSecurityHeaders_HSTSWhenEnabled(t *testing.T) {
	engine := newSecurityHeadersEngine(SecurityHeadersConfig{EnableHSTS: true})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	assert.Equal(t, "max-age=31536000; includeSubDomains", w.Header().Get(HeaderStrictTransportSecurity))
}

func TestSecurityHeaders_CustomCSPAndMaxAge(t *testing.T) {
	engine := newSecurityHeadersEngine(SecurityHeadersConfig{
		EnableHSTS:            true,
		HSTSMaxAgeSeconds:     60,
		ContentSecurityPolicy: "default-src 'self'",
	})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	assert.Equal(t, "default-src 'self'", w.Header().Get(HeaderContentSecurityPolicy))
	assert.Equal(t, "max-age=60; includeSubDomains", w.Header().Get(HeaderStrictTransportSecurity))
}

// TestSecurityHeaders_AppliedOnErrorResponse verifies headers are present even
// when a downstream handler aborts with an error status.
func TestSecurityHeaders_AppliedOnErrorResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(SecurityHeaders(SecurityHeadersConfig{}))
	engine.GET("/boom", func(c *gin.Context) { c.AbortWithStatus(http.StatusInternalServerError) })

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/boom", nil))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "nosniff", w.Header().Get(HeaderXContentTypeOptions))
	assert.Equal(t, DefaultAPIContentSecurityPolicy, w.Header().Get(HeaderContentSecurityPolicy))
}
