package apierrors

import (
	"errors"

	autherr "github.com/aspirit/xsocial/internal/domains/auth"
	posterr "github.com/aspirit/xsocial/internal/domains/post"
)

// convertDomainErrors maps domain-owned sentinel errors to API errors.
//
// Each domain declares its own sentinels (internal/domains/*/errors.go) and this
// table is the single place that decides their HTTP status and business code.
// Domains never import apierrors, so the dependency points from the HTTP edge
// inward (enforced by scripts/check-arch.sh).
func convertDomainErrors(err error) *APIError {
	switch {
	// auth
	case errors.Is(err, autherr.ErrInvalidCredentials):
		return ErrInvalidCredentials
	case errors.Is(err, autherr.ErrUsernameAlreadyExists):
		return ErrUsernameAlreadyExists
	case errors.Is(err, autherr.ErrAlreadyAuthenticated):
		return ErrAlreadyAuthenticated
	case errors.Is(err, autherr.ErrTokenExpired):
		return ErrTokenExpired
	case errors.Is(err, autherr.ErrInvalidToken), errors.Is(err, autherr.ErrRefreshTokenInvalid):
		return ErrInvalidToken
	case errors.Is(err, autherr.ErrUnauthorized):
		return ErrUnauthorized
	case errors.Is(err, autherr.ErrInvalidParams):
		return ErrInvalidParams

	// post
	case errors.Is(err, posterr.ErrPostNotFound):
		return ErrPostNotFound
	case errors.Is(err, posterr.ErrContentEmpty):
		return ErrContentEmpty
	case errors.Is(err, posterr.ErrContentTooBig), errors.Is(err, posterr.ErrInvalidParams):
		return ErrInvalidParams
	}

	return nil
}
