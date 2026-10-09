// Package request provides request-related utilities.
package request

import (
	"context"

	"github.com/aspirit/xsocial/pkg/i18n"
)

// Request context key types.
type (
	requestInfoKey struct{}
)

// RequestInfo holds all request-level information.
type RequestInfo struct {
	ClientIP  string
	Language  i18n.LanguageCode
	UserAgent string
	Referer   string
	DeviceID  string
	// Future extensions can be added here:
	// Country   string
	// Timezone  string
	// Device    string
}

// WithRequestInfo injects request information into the context.
func WithRequestInfo(ctx context.Context, info *RequestInfo) context.Context {
	return context.WithValue(ctx, requestInfoKey{}, info)
}

// RequestInfoFromContext gets request information from the context.
func RequestInfoFromContext(ctx context.Context) (*RequestInfo, bool) {
	if info, ok := ctx.Value(requestInfoKey{}).(*RequestInfo); ok {
		return info, true
	}

	return nil, false
}

// ClientIPFromContext gets client IP from the context.
// Falls back to empty string if not found.
func ClientIPFromContext(ctx context.Context) string {
	if info, ok := RequestInfoFromContext(ctx); ok {
		return info.ClientIP
	}

	return ""
}

// LanguageFromContext gets language from the context.
// Falls back to English if not found.
func LanguageFromContext(ctx context.Context) i18n.LanguageCode {
	if info, ok := RequestInfoFromContext(ctx); ok {
		return info.Language
	}

	return i18n.English
}
