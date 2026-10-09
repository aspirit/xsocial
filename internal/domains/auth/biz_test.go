package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aspirit/xsocial/internal/config"
	"github.com/aspirit/xsocial/internal/domains/auth"
	"github.com/aspirit/xsocial/internal/testutil/fake"
)

const (
	keyCurrent  = "current-signing-secret-0123456789-abcdefghij"
	keyPrevious = "previous-signing-secret-9876543210-zyxwvutsrq"
)

func newBiz(cfg config.JWTConfig) (auth.IAuthBiz, *fake.Users) {
	users := fake.NewUsers()

	return auth.NewAuthBiz(users, &fake.IDGen{}, cfg), users
}

func TestSignupLoginAndValidate(t *testing.T) {
	biz, _ := newBiz(config.JWTConfig{Key: keyCurrent})
	ctx := context.Background()

	signed, err := biz.Signup(ctx, &auth.SignupRequest{Username: "alice_01", Password: "s3cret-pass"})
	require.NoError(t, err)
	assert.Equal(t, "Bearer", signed.TokenType)

	logged, err := biz.Login(ctx, &auth.LoginRequest{Username: "alice_01", Password: "s3cret-pass"})
	require.NoError(t, err)

	user, err := biz.ValidateAndGetUser(ctx, logged.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, "alice_01", user.Username)
	assert.NotEmpty(t, user.PasswordHash, "password stored as bcrypt hash")
	assert.NotEqual(t, "s3cret-pass", user.PasswordHash)
}

func TestLogin_WrongPasswordAndUnknownUserLookTheSame(t *testing.T) {
	biz, _ := newBiz(config.JWTConfig{Key: keyCurrent})
	ctx := context.Background()
	_, err := biz.Signup(ctx, &auth.SignupRequest{Username: "alice_01", Password: "s3cret-pass"})
	require.NoError(t, err)

	_, err = biz.Login(ctx, &auth.LoginRequest{Username: "alice_01", Password: "wrong"})
	assert.ErrorIs(t, err, auth.ErrInvalidCredentials)
	_, err = biz.Login(ctx, &auth.LoginRequest{Username: "nobody_x", Password: "wrong"})
	assert.ErrorIs(t, err, auth.ErrInvalidCredentials, "no username enumeration")
}

func TestSignup_DuplicateAndInvalidUsername(t *testing.T) {
	biz, _ := newBiz(config.JWTConfig{Key: keyCurrent})
	ctx := context.Background()
	_, err := biz.Signup(ctx, &auth.SignupRequest{Username: "alice_01", Password: "s3cret-pass"})
	require.NoError(t, err)

	_, err = biz.Signup(ctx, &auth.SignupRequest{Username: "alice_01", Password: "other-pass"})
	assert.ErrorIs(t, err, auth.ErrUsernameAlreadyExists)

	_, err = biz.Signup(ctx, &auth.SignupRequest{Username: "1bad name", Password: "s3cret-pass"})
	assert.ErrorIs(t, err, auth.ErrInvalidParams)
}

func TestTokenAudienceSeparation(t *testing.T) {
	biz, _ := newBiz(config.JWTConfig{Key: keyCurrent})
	ctx := context.Background()
	tokens, err := biz.Signup(ctx, &auth.SignupRequest{Username: "alice_01", Password: "s3cret-pass"})
	require.NoError(t, err)

	_, err = biz.ValidateAndGetUser(ctx, tokens.RefreshToken)
	assert.ErrorIs(t, err, auth.ErrInvalidToken, "refresh token is not an access token")

	_, err = biz.Refresh(ctx, &auth.RefreshRequest{RefreshToken: tokens.AccessToken})
	assert.ErrorIs(t, err, auth.ErrRefreshTokenInvalid, "access token cannot be used to refresh")

	renewed, err := biz.Refresh(ctx, &auth.RefreshRequest{RefreshToken: tokens.RefreshToken})
	require.NoError(t, err)
	assert.NotEmpty(t, renewed.AccessToken)
}

func TestKeyRotation(t *testing.T) {
	ctx := context.Background()
	oldBiz, users := newBiz(config.JWTConfig{Key: keyPrevious})
	tokens, err := oldBiz.Signup(ctx, &auth.SignupRequest{Username: "alice_01", Password: "s3cret-pass"})
	require.NoError(t, err)

	// Deploy with a new key while keeping the old one for in-flight tokens.
	rotated := auth.NewAuthBiz(users, &fake.IDGen{}, config.JWTConfig{Key: keyCurrent, PreviousKeys: []string{keyPrevious}})
	_, err = rotated.ValidateAndGetUser(ctx, tokens.AccessToken)
	require.NoError(t, err, "tokens signed with a previous key stay valid during rotation")

	// Once the old key is retired, its tokens are rejected.
	retired := auth.NewAuthBiz(users, &fake.IDGen{}, config.JWTConfig{Key: keyCurrent})
	_, err = retired.ValidateAndGetUser(ctx, tokens.AccessToken)
	assert.ErrorIs(t, err, auth.ErrInvalidToken)
}

func TestExpiredToken(t *testing.T) {
	biz, _ := newBiz(config.JWTConfig{Key: keyCurrent, ParsedAccessTokenTTL: time.Nanosecond})
	ctx := context.Background()
	tokens, err := biz.Signup(ctx, &auth.SignupRequest{Username: "alice_01", Password: "s3cret-pass"})
	require.NoError(t, err)
	time.Sleep(1100 * time.Millisecond) // JWT exp has second granularity

	_, err = biz.ValidateAndGetUser(ctx, tokens.AccessToken)
	assert.ErrorIs(t, err, auth.ErrTokenExpired)
}
