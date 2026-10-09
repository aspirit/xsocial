// Package apierrors provides API error types and predefined errors.
package apierrors

import "net/http"

// APIError represents an error response for API clients.
type APIError struct {
	Code       int    `json:"code"`    // Business error code (0 for success, non-zero for errors)
	Message    string `json:"message"` // Human-readable error message
	HTTPStatus int    `json:"-"`       // HTTP status code (200, 400, 401, etc.)
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return e.Message
}

// New creates a new API error.
func New(code int, message string, httpStatus int) *APIError {
	return &APIError{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
	}
}

// Predefined API errors. Each one owns exactly one business code (codes.go).
var (
	// Bad Request (400) - Client sent invalid request.
	ErrInvalidParams = &APIError{
		Code:       CodeInvalidParams,
		Message:    "Invalid parameters",
		HTTPStatus: http.StatusBadRequest,
	}

	ErrContentEmpty = &APIError{
		Code:       CodeContentEmpty,
		Message:    "Content cannot be empty",
		HTTPStatus: http.StatusBadRequest,
	}

	// Unauthorized (401) - Authentication required.
	ErrInvalidToken = &APIError{
		Code:       CodeInvalidToken,
		Message:    "Invalid authentication token",
		HTTPStatus: http.StatusUnauthorized,
	}

	ErrTokenExpired = &APIError{
		Code:       CodeTokenExpired,
		Message:    "Authentication token expired",
		HTTPStatus: http.StatusUnauthorized,
	}

	ErrInvalidCredentials = &APIError{
		Code:       CodeInvalidCredentials,
		Message:    "Invalid credentials",
		HTTPStatus: http.StatusUnauthorized,
	}

	ErrUnauthorized = &APIError{
		Code:       CodeUnauthorized,
		Message:    "Unauthorized",
		HTTPStatus: http.StatusUnauthorized,
	}

	// Forbidden (403) - Authentication successful but access denied.
	ErrAlreadyAuthenticated = &APIError{
		Code:       CodeAlreadyAuthenticated,
		Message:    "Already authenticated",
		HTTPStatus: http.StatusForbidden,
	}

	// Not Found (404) - Resource not found.
	ErrPostNotFound = &APIError{
		Code:       CodePostNotFound,
		Message:    "Post not found",
		HTTPStatus: http.StatusNotFound,
	}

	ErrRecordNotFound = &APIError{
		Code:       CodeRecordNotFound,
		Message:    "Record not found",
		HTTPStatus: http.StatusNotFound,
	}

	// Conflict (409) - Resource conflict.
	ErrUsernameAlreadyExists = &APIError{
		Code:       CodeUsernameAlreadyExists,
		Message:    "Username already exists",
		HTTPStatus: http.StatusConflict,
	}

	ErrDuplicateRecord = &APIError{
		Code:       CodeDuplicateRecord,
		Message:    "Duplicate record",
		HTTPStatus: http.StatusConflict,
	}

	// Too Many Requests (429) - Rate limiting.
	ErrRequestLimitExceeded = &APIError{
		Code:       CodeRequestLimitExceeded,
		Message:    "Request limit exceeded",
		HTTPStatus: http.StatusTooManyRequests,
	}

	// Internal Server Error (500) - Server-side errors.
	ErrInternalError = &APIError{
		Code:       CodeInternalError,
		Message:    "Internal server error",
		HTTPStatus: http.StatusInternalServerError,
	}

	ErrDatabaseError = &APIError{
		Code:       CodeDatabaseError,
		Message:    "Database error",
		HTTPStatus: http.StatusInternalServerError,
	}

	// Not Implemented (501) - Feature not available.
	ErrNotImplemented = &APIError{
		Code:       CodeNotImplemented,
		Message:    "Feature not implemented",
		HTTPStatus: http.StatusNotImplemented,
	}
)
