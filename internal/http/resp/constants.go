// Package resp provides HTTP response utilities and constants.
package resp

// Response field constants.
const (
	// Code field name in error response (business error code).
	FieldCode = "code"

	// Message field name in error response (human-readable error message).
	FieldMessage = "message"

	// Message field name in context.
	ContextKeyMessage = "message"

	// Status field name in context (business error code).
	ContextKeyStatus = "status"
)

// ErrorResponse is the standard error envelope returned by all API endpoints.
//
//	@Description	Standard error response with business error code and human-readable message.
type ErrorResponse struct {
	Code    int    `json:"code" example:"40001"`              // Business error code (see apierrors package for full list)
	Message string `json:"message" example:"Invalid request"` // Human-readable error description
}
