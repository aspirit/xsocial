package apierrors

import (
	"errors"
	"fmt"
	"testing"

	autherr "github.com/aspirit/xsocial/internal/domains/auth"
	posterr "github.com/aspirit/xsocial/internal/domains/post"
	internalerr "github.com/aspirit/xsocial/internal/errors"
	httputil "github.com/aspirit/xsocial/pkg/http"
)

const errMsgConversionNil = "Expected error conversion, got nil"

func assertConvert(t *testing.T, err error, expected *APIError) {
	t.Helper()
	result := Convert(err)
	if result == nil {
		t.Fatal(errMsgConversionNil)
	}
	if result.Code != expected.Code {
		t.Errorf("Code: expected %d, got %d", expected.Code, result.Code)
	}
	if result.HTTPStatus != expected.HTTPStatus {
		t.Errorf("HTTPStatus: expected %d, got %d", expected.HTTPStatus, result.HTTPStatus)
	}
	if result.Message != expected.Message {
		t.Errorf("Message: expected %q, got %q", expected.Message, result.Message)
	}
}

// TestConvertDomain pins the domain-error → API-error table: changing a status
// code or business code is a breaking API change and must fail here first.
func TestConvertDomain(t *testing.T) {
	tests := []struct {
		err      error
		expected *APIError
	}{
		{autherr.ErrInvalidCredentials, ErrInvalidCredentials},
		{autherr.ErrUsernameAlreadyExists, ErrUsernameAlreadyExists},
		{autherr.ErrAlreadyAuthenticated, ErrAlreadyAuthenticated},
		{autherr.ErrTokenExpired, ErrTokenExpired},
		{autherr.ErrInvalidToken, ErrInvalidToken},
		{autherr.ErrRefreshTokenInvalid, ErrInvalidToken},
		{autherr.ErrUnauthorized, ErrUnauthorized},
		{autherr.ErrInvalidParams, ErrInvalidParams},
		{posterr.ErrPostNotFound, ErrPostNotFound},
		{posterr.ErrContentEmpty, ErrContentEmpty},
		{posterr.ErrContentTooBig, ErrInvalidParams},
		{posterr.ErrInvalidParams, ErrInvalidParams},
	}
	for _, tt := range tests {
		t.Run(tt.err.Error(), func(t *testing.T) {
			assertConvert(t, tt.err, tt.expected)
		})
	}
}

func TestConvertValidation(t *testing.T) {
	tests := []struct {
		err      error
		expected *APIError
	}{
		{internalerr.ErrInvalidParams, ErrInvalidParams},
		{httputil.ErrInvalidParams, ErrInvalidParams},
		{internalerr.ErrRequestLimitExceeded, ErrRequestLimitExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.err.Error(), func(t *testing.T) {
			assertConvert(t, tt.err, tt.expected)
		})
	}
}

func TestConvertBusiness(t *testing.T) {
	assertConvert(t, internalerr.ErrNotImplemented, ErrNotImplemented)
}

func TestConvertStorage(t *testing.T) {
	assertConvert(t, internalerr.ErrRecordNotFound, ErrRecordNotFound)
}

func TestConvertWrappedError(t *testing.T) {
	// errors.Is must still match through %w wrapping added by lower layers.
	wrapped := fmt.Errorf("handler: %w", fmt.Errorf("biz: %w", posterr.ErrPostNotFound))
	assertConvert(t, wrapped, ErrPostNotFound)
}

func TestConvertUnknownIsInternal(t *testing.T) {
	// Unmapped errors never leak their text to clients.
	assertConvert(t, errors.New("db exploded: secret detail"), ErrInternalError)
}

func TestConvertWithContext(t *testing.T) {
	// Test with unhandled error so Convert returns ErrInternalError and context is applied
	err := errors.New("some unhandled error")
	context := "test context"

	result := ConvertWithContext(err, context)

	if result == nil {
		t.Fatal(errMsgConversionNil)
	}

	expectedMessage := "Internal error: test context"
	if result.Message != expectedMessage {
		t.Errorf("Expected message %s, got %s", expectedMessage, result.Message)
	}

	// Test with domain error that should not get context
	domainErr := posterr.ErrPostNotFound
	result2 := ConvertWithContext(domainErr, context)

	if result2 == nil {
		t.Fatal(errMsgConversionNil)
	}

	// Should return the same as Convert without context
	expectedResult := Convert(domainErr)
	if result2.Code != expectedResult.Code {
		t.Errorf("Expected code %d, got %d", expectedResult.Code, result2.Code)
	}
}

func TestIsClientError(t *testing.T) {
	tests := []struct {
		name     string
		err      *APIError
		expected bool
	}{
		{
			name:     "Client error (400)",
			err:      &APIError{Code: 4000, HTTPStatus: 400},
			expected: true,
		},
		{
			name:     "Server error (500)",
			err:      &APIError{Code: 5000, HTTPStatus: 500},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsClientError(tt.err)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsServerError(t *testing.T) {
	tests := []struct {
		name     string
		err      *APIError
		expected bool
	}{
		{
			name:     "Client error (400)",
			err:      &APIError{Code: 4000, HTTPStatus: 400},
			expected: false,
		},
		{
			name:     "Server error (500)",
			err:      &APIError{Code: 5000, HTTPStatus: 500},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsServerError(tt.err)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}
