// Package apierrors provides conversion from internal errors to API errors.
package apierrors

import (
	"errors"
	"fmt"
	"strings"

	internalerr "github.com/aspirit/xsocial/internal/errors"
	httputil "github.com/aspirit/xsocial/pkg/http"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
)

// Convert converts internal errors to API errors suitable for HTTP responses.
// Domain, validation and storage errors map to a stable {code, message}
// contract; anything unmapped becomes a generic 500, so no internal detail
// ever reaches a client.
func Convert(err error) *APIError {
	if err == nil {
		return nil
	}

	if apiErr := convertInternalErrors(err); apiErr != nil {
		return apiErr
	}

	if apiErr := convertMongoErrors(err); apiErr != nil {
		return apiErr
	}

	// Log the original error for debugging purposes
	// but return a generic error to avoid information leakage.
	zap.L().Error("Unhandled internal error", zap.Error(err))

	return ErrInternalError
}

// convertInternalErrors handles conversion of internal business logic errors.
func convertInternalErrors(err error) *APIError {
	// Domain errors first, so a domain sentinel is never swallowed by the
	// generic pattern matching on Gin binding errors.
	if apiErr := convertDomainErrors(err); apiErr != nil {
		return apiErr
	}

	if apiErr := convertGinErrors(err); apiErr != nil {
		return apiErr
	}

	if apiErr := convertStorageErrors(err); apiErr != nil {
		return apiErr
	}

	if apiErr := convertValidationErrors(err); apiErr != nil {
		return apiErr
	}

	switch {
	case errors.Is(err, internalerr.ErrNotImplemented):
		return ErrNotImplemented
	case errors.Is(err, internalerr.ErrInternalError):
		return ErrInternalError
	}

	return nil
}

// convertGinErrors handles Gin framework specific errors.
func convertGinErrors(err error) *APIError {
	// Check for Gin binding errors
	if ginErr, ok := err.(gin.Error); ok {
		switch ginErr.Type {
		case gin.ErrorTypeBind:
			return ErrInvalidParams
		case gin.ErrorTypeRender:
			return ErrInternalError
		case gin.ErrorTypePublic:
			return ErrInvalidParams
		}
	}

	// Check for common Gin binding error patterns
	// Note: Only match specific Gin binding error patterns to avoid false positives
	// with domain-specific errors that may contain words like "required"
	errStr := err.Error()
	if strings.Contains(errStr, "binding") ||
		(strings.Contains(errStr, "required") && strings.Contains(errStr, "field")) {
		return ErrInvalidParams
	}

	return nil
}

// databaseErrors are storage-layer failures that all surface as one generic
// database error: the client learns nothing about the deployment.
var databaseErrors = []error{
	internalerr.ErrDatabaseNotInitialized,
	internalerr.ErrMongoConfigNil,
	internalerr.ErrMongoURIEmpty,
	internalerr.ErrPoolSizeNegative,
	internalerr.ErrMinPoolSizeNegative,
	internalerr.ErrMinPoolSizeGreaterThanMax,
	internalerr.ErrMaxIdleTimeNegative,
	internalerr.ErrConnectTimeoutNegative,
	internalerr.ErrSocketTimeoutNegative,
	internalerr.ErrServerSelectionTimeoutNegative,
	internalerr.ErrInvalidReadPreference,
	internalerr.ErrInvalidWriteConcern,
	internalerr.ErrInvalidReadConcern,
	internalerr.ErrInvalidCompressor,
	internalerr.ErrRedisClientNotFound,
	internalerr.ErrClosingRedisClients,
}

// convertStorageErrors handles storage layer errors.
func convertStorageErrors(err error) *APIError {
	switch {
	case errors.Is(err, internalerr.ErrRecordNotFound):
		return ErrRecordNotFound
	case errors.Is(err, internalerr.ErrDuplicateRecord):
		return ErrDuplicateRecord
	}

	for _, dbErr := range databaseErrors {
		if errors.Is(err, dbErr) {
			return ErrDatabaseError
		}
	}

	return nil
}

// convertValidationErrors handles validation specific errors.
func convertValidationErrors(err error) *APIError {
	switch {
	case errors.Is(err, httputil.ErrInvalidParams), errors.Is(err, internalerr.ErrInvalidParams):
		return ErrInvalidParams
	case errors.Is(err, internalerr.ErrRequestLimitExceeded):
		return ErrRequestLimitExceeded
	}

	return nil
}

// convertMongoErrors handles MongoDB driver specific errors.
func convertMongoErrors(err error) *APIError {
	// Check if this is an ID generation error (should be 500, not 404)
	errMsg := err.Error()
	if strings.Contains(errMsg, "failed to generate") && strings.Contains(errMsg, "ID") {
		// ID generation failures are internal server errors, not "record not found"
		return ErrInternalError
	}

	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrRecordNotFound
	}

	return nil
}

// ConvertWithContext converts an internal error to an API error with additional context.
// This is useful when you want to provide more specific error messages while still
// maintaining the security benefits of error conversion.
func ConvertWithContext(err error, context string) *APIError {
	apiErr := Convert(err)
	if apiErr == ErrInternalError && context != "" {
		// For internal errors, we can provide context without exposing details
		return &APIError{
			Code:       CodeInternalError,
			Message:    fmt.Sprintf("Internal error: %s", context),
			HTTPStatus: 500,
		}
	}

	return apiErr
}

// IsClientError returns true if the error is a client error (4xx).
func IsClientError(apiErr *APIError) bool {
	return apiErr != nil && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500
}

// IsServerError returns true if the error is a server error (5xx).
func IsServerError(apiErr *APIError) bool {
	return apiErr != nil && apiErr.HTTPStatus >= 500
}
