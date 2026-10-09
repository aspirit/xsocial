// Package errors provides validation error definitions.
package errors

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Validation errors.
var (
	// Request validation.
	ErrInvalidParams        = errors.New("invalid parameters")
	ErrRequestLimitExceeded = errors.New("request limit exceeded")

	// Rate-limit configuration validation.
	ErrRateLimitCacheAddress    = errors.New("ratelimit storage is cache but cache backend address is empty")
	ErrRateLimitStorage         = errors.New("ratelimit storage must be memory or cache")
	ErrRateLimitGlobalReq       = errors.New("ratelimit global requests must be positive")
	ErrRateLimitPolicyName      = errors.New("ratelimit policy name required")
	ErrRateLimitPolicyReq       = errors.New("ratelimit policy requests must be positive")
	ErrRateLimitPolicyUnknown   = errors.New("ratelimit policy name unknown")
	ErrRateLimitDisabledRelease = errors.New("rate limiting must be enabled in release mode (set ratelimit.enabled=true)")

	// JWT signing key validation.
	ErrJWTKeyEmpty      = errors.New("jwt signing key is required")
	ErrJWTKeyTooShort   = errors.New("jwt signing key too short: need at least 32 bytes of high-entropy secret (e.g. openssl rand -base64 48)")
	ErrJWTKeyLowEntropy = errors.New("jwt signing key entropy too low: use a random secret, not a guessable passphrase")
)

// HandleJSONBindingError handles JSON binding errors and converts them to user-friendly messages.
// This function provides consistent error handling for JSON binding errors across all handlers.
//
// It handles:
//   - JSON type errors (invalid type for a field)
//   - JSON syntax errors (malformed JSON)
//   - Binding validation errors (missing required fields, validation failures)
//
// Example usage:
//
//	if err := c.BindJSON(&request); err != nil {
//	    return errors.HandleJSONBindingError(err)
//	}
func HandleJSONBindingError(err error) error {
	// Check for JSON type errors and provide specific field information
	var unmarshalTypeError *json.UnmarshalTypeError
	if errors.As(err, &unmarshalTypeError) {
		return fmt.Errorf("%w: invalid type for field %s", ErrInvalidParams, unmarshalTypeError.Field)
	}

	// Check for JSON syntax errors
	if strings.Contains(err.Error(), "invalid character") ||
		strings.Contains(err.Error(), "expect :") ||
		strings.Contains(err.Error(), "unexpected end of JSON input") ||
		strings.Contains(err.Error(), "ReadMapCB") {
		return fmt.Errorf("%w: invalid JSON format", ErrInvalidParams)
	}

	// Check for binding validation errors
	if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "binding") {
		return fmt.Errorf("%w: %s", ErrInvalidParams, err.Error())
	}

	return fmt.Errorf("%w: invalid request parameters", ErrInvalidParams)
}
