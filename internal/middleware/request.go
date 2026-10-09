// Package middleware provides HTTP middleware functions.
package middleware

import (
	"net/http"

	httputil "github.com/aspirit/xsocial/pkg/http"
	"github.com/aspirit/xsocial/pkg/i18n"
	"github.com/aspirit/xsocial/pkg/request"
	"github.com/gin-gonic/gin"
)

// RequestContextDetector is a unified middleware that extracts all request-level information
// and sets it in the request context.
func RequestContextDetector() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract all request information
		requestInfo := &request.RequestInfo{
			ClientIP:  c.ClientIP(),
			Language:  detectLanguageFromHeader(c.Request),
			UserAgent: c.GetHeader(httputil.HeaderUserAgent),
			Referer:   c.Request.Referer(),
			DeviceID:  c.GetHeader(httputil.HeaderDeviceID),
		}

		// Set request info in context
		ctx := request.WithRequestInfo(c.Request.Context(), requestInfo)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// detectLanguageFromHeader detects the user's language preference from Accept-Language header.
// Falls back to English if no valid language is found.
func detectLanguageFromHeader(r *http.Request) i18n.LanguageCode {
	acceptLang := r.Header.Get(httputil.HeaderAcceptLanguage)
	if acceptLang != "" {
		return i18n.ParseAcceptLanguage(acceptLang)
	}

	// Default to English
	return i18n.English
}
