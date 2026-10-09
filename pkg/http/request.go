// Package http provides HTTP request handling utilities and standard constants.
//
// This package contains:
// - HTTP header constants (User-Agent, Authorization, etc.)
// - Request helper utilities
// - Bearer token handling
//
// The constants in this package are designed to be reusable across different projects
// and follow HTTP protocol standards.
package http

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// ErrInvalidParams indicates a malformed request parameter.
var ErrInvalidParams = errors.New("invalid parameters")

// HTTP header and token type constants.
//
// These constants follow HTTP protocol standards and can be used across different projects.
// Example usage:
//
//	c.GetHeader(http.HeaderUserAgent)
//	c.GetHeader(http.HeaderAuthorization)
const (
	// HTTP header names.
	HeaderUserAgent      = "User-Agent"
	HeaderAuthorization  = "Authorization"
	HeaderContentType    = "Content-Type"
	HeaderDeviceID       = "X-Device-ID"
	HeaderAcceptLanguage = "Accept-Language"

	// OAuth2 token type constants.
	TokenTypeBearer    = "Bearer"              // OAuth2 standard token type (without space)
	HeaderBearerPrefix = TokenTypeBearer + " " // HTTP Authorization header prefix (with space)
)

// RequestHelper provides utilities for handling HTTP requests.
type RequestHelper interface {
	// BindJSON binds JSON request body to the given object
	BindJSON(c *gin.Context, obj any) error

	// GetBearerToken extracts Bearer token from Authorization header
	GetBearerToken(c *gin.Context) string

	// SetBearerToken sets Bearer token in Authorization header
	SetBearerToken(c *gin.Context, token string)
}

// requestHelper is the default implementation of RequestHelper.
type requestHelper struct{}

// NewRequestHelper creates a new RequestHelper instance.
func NewRequestHelper() RequestHelper {
	return &requestHelper{}
}

// BindJSON binds JSON request body to the given object.
func (h *requestHelper) BindJSON(c *gin.Context, obj any) error {
	if err := c.ShouldBindJSON(obj); err != nil {
		return fmt.Errorf("failed to bind JSON request: %w", err)
	}

	return nil
}

// ParseUint64ID parses a string ID (e.g. from URL param) to uint64.
// Empty string returns ErrInvalidParams. Use for path params or single required ID.
func ParseUint64ID(idStr string) (uint64, error) {
	return ParseRequiredUint64ID(idStr, "id")
}

// ParseRequiredUint64ID parses a required ID string to uint64 (e.g. from JSON body).
// Empty or invalid string returns error with field name. Use for required body/query fields.
func ParseRequiredUint64ID(s, fieldName string) (uint64, error) {
	if s == "" {
		return 0, fmt.Errorf("%s is required: %w", fieldName, ErrInvalidParams)
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid numeric string: %w", fieldName, ErrInvalidParams)
	}

	return n, nil
}

// ParseOptionalUint64ID parses an optional ID string to *uint64 (e.g. from JSON body).
// Empty string returns (nil, nil). Invalid non-empty string returns error.
func ParseOptionalUint64ID(s string) (*uint64, error) {
	if s == "" {
		return nil, nil
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid id string: %w", ErrInvalidParams)
	}

	return &n, nil
}

// GetBearerToken extracts Bearer token from Authorization header.
func (h *requestHelper) GetBearerToken(c *gin.Context) string {
	authHeader := c.GetHeader(HeaderAuthorization)

	if strings.HasPrefix(authHeader, HeaderBearerPrefix) {
		return strings.TrimPrefix(authHeader, HeaderBearerPrefix)
	}

	return ""
}

// SetBearerToken sets Bearer token in Authorization header.
func (h *requestHelper) SetBearerToken(c *gin.Context, token string) {
	c.Request.Header.Set(HeaderAuthorization, HeaderBearerPrefix+token)
}
