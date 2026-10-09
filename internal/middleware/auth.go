// Package middleware provides authentication middleware for Gin web framework.
//
// This file contains a comprehensive authentication middleware that integrates with
// the AuthBiz layer to provide JWT-based authentication for API endpoints.
//
// Key Features:
//   - Three authentication modes: Required, Optional, Forbidden
//   - Integration with AuthBiz layer for authentication logic
//   - Easy testing with mockable interfaces
//   - Context-aware logging that inherits request context
//
// Authentication Modes:
//  1. Required  - User must be authenticated (401 if not)
//  2. Optional  - Enhanced experience for authenticated users, allows anonymous
//  3. Forbidden - Blocks authenticated users (useful for login/register pages)
//
// Usage Examples:
//
//	// Basic setup - no logger dependency needed
//	authMiddleware := middleware.NewAuthMiddleware(deps.AuthBiz)
//
//	// Router groups with different modes
//	private.Use(authMiddleware.Required())   // Protected routes
//	public.Use(authMiddleware.Optional())    // Enhanced public routes
//	guest.Use(authMiddleware.Forbidden())    // Guest-only routes
//
// Thread Safety:
//
//	This middleware is thread-safe and can be used concurrently across multiple
//	goroutines and HTTP requests.
package middleware

import (
	"context"
	"strings"

	autherr "github.com/aspirit/xsocial/internal/domains/auth"
	"github.com/aspirit/xsocial/internal/http/resp"
	"github.com/aspirit/xsocial/internal/model"
	httputil "github.com/aspirit/xsocial/pkg/http"
	"github.com/aspirit/xsocial/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ============================================================================
// Auth Provider Interface
// ============================================================================

// AuthProvider defines the interface for authentication operations.
type AuthProvider interface {
	// Token parsing and validation
	ValidateToken(ctx context.Context, token string) (bool, error)
	GetUserInfoFromToken(ctx context.Context, token string) (*model.UserInfo, error)
	// ValidateAndGetUser validates access token and returns user in one parse (preferred for middleware).
	ValidateAndGetUser(ctx context.Context, token string) (*model.UserInfo, error)

	// Context management
	SetUserContext(c *gin.Context, userInfo *model.UserInfo)
	GetCurrentUser(c *gin.Context) (*model.UserInfo, bool)
	IsAuthenticated(c *gin.Context) bool
	RequireAuth(c *gin.Context) (*model.UserInfo, error)
}

// ============================================================================
// Auth Middleware
// ============================================================================

// AuthMiddleware provides authentication middleware functionality.
type AuthMiddleware struct {
	authProvider AuthProvider
}

// NewAuthMiddleware creates a new authentication middleware
// The logger is now obtained from the request context, allowing it to inherit
// request-specific information like request ID, user ID, etc.
func NewAuthMiddleware(authProvider AuthProvider) *AuthMiddleware {
	return &AuthMiddleware{
		authProvider: authProvider,
	}
}

// Required creates middleware that requires authentication
// Returns 401 Unauthorized if user is not authenticated.
func (am *AuthMiddleware) Required() gin.HandlerFunc {
	return func(c *gin.Context) {
		log := logger.LoggerFromContext(c.Request.Context())

		// Step 1: Extract token from Authorization header
		token := extractTokenFromHeader(c)
		if token == "" {
			log.Debug("Authentication required but no token provided")
			resp.Fail(c, autherr.ErrUnauthorized)
			c.Abort()

			return
		}

		// Step 2: Validate token and get user in a single parse
		userInfo, err := am.authProvider.ValidateAndGetUser(c.Request.Context(), token)
		if err != nil || userInfo == nil {
			log.Debug("Authentication required but token is invalid", zap.Error(err))
			resp.Fail(c, autherr.ErrUnauthorized)
			c.Abort()

			return
		}

		// Step 3: Set user context for subsequent handlers
		am.authProvider.SetUserContext(c, userInfo)
		log.Debug("User authenticated successfully", zap.Uint64("user_id", userInfo.ID))

		c.Next()
	}
}

// Optional creates middleware that allows both authenticated and anonymous users
// Authenticated users get enhanced functionality, anonymous users are allowed to proceed.
func (am *AuthMiddleware) Optional() gin.HandlerFunc {
	return func(c *gin.Context) {
		log := logger.LoggerFromContext(c.Request.Context())

		// Step 1: Extract token from Authorization header
		token := extractTokenFromHeader(c)
		if token == "" {
			log.Debug("User is anonymous in optional middleware")
			c.Next()

			return
		}

		// Step 2: Validate token and get user in a single parse
		userInfo, err := am.authProvider.ValidateAndGetUser(c.Request.Context(), token)
		if err != nil || userInfo == nil {
			log.Debug("Token is invalid in optional middleware", zap.Error(err))
			c.Next()

			return
		}

		// Step 3: Set user context for subsequent handlers
		am.authProvider.SetUserContext(c, userInfo)
		log.Debug("User is authenticated in optional middleware", zap.Uint64("user_id", userInfo.ID))

		c.Next()
	}
}

// Forbidden creates middleware that blocks authenticated users
// Used for endpoints like registration and login where authenticated users should be redirected.
func (am *AuthMiddleware) Forbidden() gin.HandlerFunc {
	return func(c *gin.Context) {
		log := logger.LoggerFromContext(c.Request.Context())

		// Check if user is authenticated by validating token and getting user in a single parse
		token := extractTokenFromHeader(c)
		if token != "" {
			userInfo, err := am.authProvider.ValidateAndGetUser(c.Request.Context(), token)
			if err == nil && userInfo != nil {
				log.Info("Authenticated user attempted to access forbidden endpoint", zap.Uint64("user_id", userInfo.ID))
				resp.Fail(c, autherr.ErrAlreadyAuthenticated)
				c.Abort()

				return
			}
		}

		c.Next()
	}
}

// extractTokenFromHeader extracts the Bearer token from the Authorization header
// Returns the token string if present, otherwise returns an empty string.
func extractTokenFromHeader(c *gin.Context) string {
	authHeader := c.GetHeader(httputil.HeaderAuthorization)
	bearerPrefix := httputil.HeaderBearerPrefix
	if len(authHeader) > len(bearerPrefix) &&
		(authHeader[:len(bearerPrefix)] == bearerPrefix ||
			strings.EqualFold(authHeader[:len(bearerPrefix)], bearerPrefix)) {
		return strings.TrimSpace(authHeader[len(bearerPrefix):])
	}

	return ""
}
