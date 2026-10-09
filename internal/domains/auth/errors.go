// Package auth implements username/password authentication with EdDSA JWTs.
package auth

import "errors"

// Auth domain errors. internal/apierrors maps them to API error codes, so the
// HTTP layer never leaks internal error text.
var (
	ErrInvalidParams         = errors.New("invalid auth parameters")
	ErrInvalidCredentials    = errors.New("invalid credentials")
	ErrUsernameAlreadyExists = errors.New("username already exists")
	ErrUnauthorized          = errors.New("unauthorized")
	ErrAlreadyAuthenticated  = errors.New("already authenticated")
	ErrInvalidToken          = errors.New("invalid token")
	ErrTokenExpired          = errors.New("token expired")
	ErrRefreshTokenInvalid   = errors.New("refresh token is invalid")
)
