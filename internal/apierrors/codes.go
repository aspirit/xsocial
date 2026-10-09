// Package apierrors provides API-specific error definitions for HTTP responses.
// These errors are designed to be consumed by frontend clients and external APIs.
package apierrors

// Business error codes, returned next to the HTTP status in every error
// response ({code, message}). They are a client contract: codes are grouped by
// HTTP class (1xxx = 400, 2xxx = 401, ...) and are never renumbered once published.
const (
	// Success - No business error code needed when HTTP status is 200.
	CodeSuccess = 0

	// Bad Request (400) - Client sent invalid request.
	CodeInvalidParams = 1001 // Invalid parameters
	CodeContentEmpty  = 1002 // Content cannot be empty

	// Unauthorized (401) - Authentication required.
	CodeInvalidToken       = 2001 // Invalid authentication token
	CodeTokenExpired       = 2002 // Authentication token expired
	CodeInvalidCredentials = 2003 // Invalid credentials
	CodeUnauthorized       = 2004 // Unauthorized access

	// Forbidden (403) - Authentication successful but access denied.
	CodeAlreadyAuthenticated = 3001 // Already authenticated (guest-only route)

	// Not Found (404) - Resource not found.
	CodePostNotFound   = 4001 // Post not found
	CodeRecordNotFound = 4002 // Generic record not found

	// Conflict (409) - Resource conflict.
	CodeUsernameAlreadyExists = 5001 // Username already exists
	CodeDuplicateRecord       = 5002 // Duplicate record

	// Too Many Requests (429) - Rate limiting.
	CodeRequestLimitExceeded = 6001 // Rate limit exceeded

	// Internal Server Error (500) - Server-side errors.
	CodeInternalError = 7001 // Generic server error
	CodeDatabaseError = 7002 // Database error

	// Not Implemented (501) - Feature not available.
	CodeNotImplemented = 8001 // Feature not implemented
)
