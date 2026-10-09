package auth

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/aspirit/xsocial/internal/config"
	internalerr "github.com/aspirit/xsocial/internal/errors"
	"github.com/aspirit/xsocial/internal/model"
	"github.com/aspirit/xsocial/internal/storage/interfaces"
	"github.com/aspirit/xsocial/pkg/idgen"
	"github.com/aspirit/xsocial/pkg/utils"
)

const (
	audienceAccess  = "access"
	audienceRefresh = "refresh"
	tokenTypeBearer = "Bearer"

	defaultAccessTTL  = time.Hour
	defaultRefreshTTL = 30 * 24 * time.Hour

	// ctxUserKey is the gin context key holding the authenticated *model.UserInfo.
	ctxUserKey = "auth.user"
)

// IAuthBiz is the auth domain contract. It also satisfies
// middleware.AuthProvider, which is how the HTTP layer authenticates requests.
type IAuthBiz interface {
	Signup(ctx context.Context, req *SignupRequest) (*TokenResponse, error)
	Login(ctx context.Context, req *LoginRequest) (*TokenResponse, error)
	Refresh(ctx context.Context, req *RefreshRequest) (*TokenResponse, error)

	ValidateToken(ctx context.Context, token string) (bool, error)
	GetUserInfoFromToken(ctx context.Context, token string) (*model.UserInfo, error)
	ValidateAndGetUser(ctx context.Context, token string) (*model.UserInfo, error)

	SetUserContext(c *gin.Context, userInfo *model.UserInfo)
	GetCurrentUser(c *gin.Context) (*model.UserInfo, bool)
	GetCurrentUID(c *gin.Context) (uint64, bool)
	IsAuthenticated(c *gin.Context) bool
	RequireAuth(c *gin.Context) (*model.UserInfo, error)
}

// AuthBiz implements IAuthBiz.
type AuthBiz struct {
	users interfaces.IUserInfoRepository
	idGen idgen.Generator
	jwt   config.JWTConfig
}

// NewAuthBiz creates the auth domain service.
func NewAuthBiz(users interfaces.IUserInfoRepository, idGen idgen.Generator, jwtCfg config.JWTConfig) IAuthBiz {
	return &AuthBiz{users: users, idGen: idGen, jwt: jwtCfg}
}

// Signup creates an account and returns a token pair.
func (b *AuthBiz) Signup(ctx context.Context, req *SignupRequest) (*TokenResponse, error) {
	username := strings.TrimSpace(req.Username)
	if !config.BusinessValidator.IsUsername(username) {
		return nil, ErrInvalidParams
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	id, err := b.idGen.NextID()
	if err != nil {
		return nil, fmt.Errorf("generate user id: %w", err)
	}

	user := model.NewUserInfo(id, username, string(hash))
	if err := b.users.Create(ctx, user); err != nil {
		// The unique username index turns a concurrent duplicate signup into a
		// duplicate-key error instead of a race.
		if errors.Is(err, internalerr.ErrDuplicateRecord) {
			return nil, ErrUsernameAlreadyExists
		}

		return nil, fmt.Errorf("insert user: %w", err)
	}

	return b.issueTokens(user)
}

// Login verifies credentials and returns a token pair.
func (b *AuthBiz) Login(ctx context.Context, req *LoginRequest) (*TokenResponse, error) {
	user, err := b.users.FindByUsername(ctx, strings.TrimSpace(req.Username))
	if err != nil {
		if errors.Is(err, internalerr.ErrRecordNotFound) {
			return nil, ErrInvalidCredentials
		}

		return nil, fmt.Errorf("find user: %w", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		return nil, ErrInvalidCredentials
	}

	return b.issueTokens(user)
}

// Refresh exchanges a valid refresh token for a new token pair.
func (b *AuthBiz) Refresh(ctx context.Context, req *RefreshRequest) (*TokenResponse, error) {
	claims, err := b.parse(req.RefreshToken, audienceRefresh)
	if err != nil {
		return nil, ErrRefreshTokenInvalid
	}
	user, err := b.userFromClaims(ctx, claims)
	if err != nil {
		return nil, ErrRefreshTokenInvalid
	}

	return b.issueTokens(user)
}

// ValidateToken reports whether an access token is valid.
func (b *AuthBiz) ValidateToken(_ context.Context, token string) (bool, error) {
	if _, err := b.parse(token, audienceAccess); err != nil {
		return false, err
	}

	return true, nil
}

// GetUserInfoFromToken resolves the user behind an access token.
func (b *AuthBiz) GetUserInfoFromToken(ctx context.Context, token string) (*model.UserInfo, error) {
	return b.ValidateAndGetUser(ctx, token)
}

// ValidateAndGetUser validates an access token and loads its user in one step
// (used by the auth middleware on every authenticated request).
func (b *AuthBiz) ValidateAndGetUser(ctx context.Context, token string) (*model.UserInfo, error) {
	claims, err := b.parse(token, audienceAccess)
	if err != nil {
		return nil, err
	}

	return b.userFromClaims(ctx, claims)
}

// SetUserContext stores the authenticated user on the request context.
func (b *AuthBiz) SetUserContext(c *gin.Context, userInfo *model.UserInfo) {
	c.Set(ctxUserKey, userInfo)
}

// GetCurrentUser returns the authenticated user, if any.
func (b *AuthBiz) GetCurrentUser(c *gin.Context) (*model.UserInfo, bool) {
	v, ok := c.Get(ctxUserKey)
	if !ok {
		return nil, false
	}
	user, ok := v.(*model.UserInfo)

	return user, ok && user != nil
}

// GetCurrentUID returns the authenticated user's ID, if any.
func (b *AuthBiz) GetCurrentUID(c *gin.Context) (uint64, bool) {
	user, ok := b.GetCurrentUser(c)
	if !ok {
		return 0, false
	}

	return user.ID, true
}

// IsAuthenticated reports whether the request is authenticated.
func (b *AuthBiz) IsAuthenticated(c *gin.Context) bool {
	_, ok := b.GetCurrentUser(c)

	return ok
}

// RequireAuth returns the user or ErrUnauthorized.
func (b *AuthBiz) RequireAuth(c *gin.Context) (*model.UserInfo, error) {
	user, ok := b.GetCurrentUser(c)
	if !ok {
		return nil, ErrUnauthorized
	}

	return user, nil
}

// issueTokens mints an access + refresh token pair signed with the current key.
// Each token carries a kid header so verification can pick the right key during
// a rotation window (secrets.jwt.previous_keys).
func (b *AuthBiz) issueTokens(user *model.UserInfo) (*TokenResponse, error) {
	now := time.Now()
	accessTTL := orDefault(b.jwt.ParsedAccessTokenTTL, defaultAccessTTL)
	refreshTTL := orDefault(b.jwt.ParsedRefreshTokenTTL, defaultRefreshTTL)
	sub := strconv.FormatUint(user.ID, 10)

	access, err := utils.JwtUtil.GenerateToken(jwt.RegisteredClaims{
		Subject:   sub,
		Audience:  jwt.ClaimStrings{audienceAccess},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(accessTTL)),
	}, b.jwt.Key)
	if err != nil {
		return nil, fmt.Errorf("sign access token: %w", err)
	}
	refresh, err := utils.JwtUtil.GenerateToken(jwt.RegisteredClaims{
		Subject:   sub,
		Audience:  jwt.ClaimStrings{audienceRefresh},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(refreshTTL)),
	}, b.jwt.Key)
	if err != nil {
		return nil, fmt.Errorf("sign refresh token: %w", err)
	}

	return &TokenResponse{
		UID:              sub,
		Username:         user.Username,
		TokenType:        tokenTypeBearer,
		AccessToken:      access,
		ExpiresAt:        now.Add(accessTTL).UnixMilli(),
		RefreshToken:     refresh,
		RefreshExpiresAt: now.Add(refreshTTL).UnixMilli(),
	}, nil
}

// parse verifies signature (current or previous keys), expiry and audience.
func (b *AuthBiz) parse(token, audience string) (*jwt.RegisteredClaims, error) {
	keys := append([]string{b.jwt.Key}, b.jwt.PreviousKeys...)
	claims, err := utils.JwtUtil.ParseTokenWithKeys(token, keys...)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}

		return nil, ErrInvalidToken
	}
	for _, aud := range claims.Audience {
		if aud == audience {
			return claims, nil
		}
	}

	return nil, ErrInvalidToken
}

func (b *AuthBiz) userFromClaims(ctx context.Context, claims *jwt.RegisteredClaims) (*model.UserInfo, error) {
	uid, err := strconv.ParseUint(claims.Subject, 10, 64)
	if err != nil {
		return nil, ErrInvalidToken
	}
	user, err := b.users.FindByID(ctx, uid)
	if err != nil {
		if errors.Is(err, internalerr.ErrRecordNotFound) {
			return nil, ErrUnauthorized
		}

		return nil, fmt.Errorf("load user: %w", err)
	}

	return user, nil
}

func orDefault(d, fallback time.Duration) time.Duration {
	if d > 0 {
		return d
	}

	return fallback
}
