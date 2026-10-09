package utils_test

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/aspirit/xsocial/pkg/utils"
	"github.com/golang-jwt/jwt/v5"
)

func sampleClaims() jwt.RegisteredClaims {
	return jwt.RegisteredClaims{
		Subject:   "user123",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
}

func tokenHeaderKid(t *testing.T, token string) string {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("malformed token: %d parts", len(parts))
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[0], "="))
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	var h struct {
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &h); err != nil {
		t.Fatalf("parse header: %v", err)
	}

	return h.Kid
}

func TestJwtUtil_GenerateToken_StampsKid(t *testing.T) {
	const key = "test-secret-key"
	token, err := utils.JwtUtil.GenerateToken(sampleClaims(), key)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}
	kid := tokenHeaderKid(t, token)
	if kid == "" {
		t.Fatal("expected kid header, got empty")
	}
	if kid != utils.KidForKey(key) {
		t.Fatalf("kid mismatch: header %q vs KidForKey %q", kid, utils.KidForKey(key))
	}
}

func TestJwtUtil_ParseTokenWithKeys_Rotation(t *testing.T) {
	const oldKey = "old-signing-key-aaaaaaaaaaaaaaaa"
	const newKey = "new-signing-key-bbbbbbbbbbbbbbbb"

	// Token minted with the now-retired key must still validate while it is kept
	// in the parse set (the rotation window).
	oldToken, err := utils.JwtUtil.GenerateToken(sampleClaims(), oldKey)
	if err != nil {
		t.Fatalf("GenerateToken(oldKey): %v", err)
	}
	if _, err := utils.JwtUtil.ParseTokenWithKeys(oldToken, newKey, oldKey); err != nil {
		t.Fatalf("expected old token to validate during rotation, got %v", err)
	}

	// Token minted with the current key validates with the same key set.
	newToken, err := utils.JwtUtil.GenerateToken(sampleClaims(), newKey)
	if err != nil {
		t.Fatalf("GenerateToken(newKey): %v", err)
	}
	if _, err := utils.JwtUtil.ParseTokenWithKeys(newToken, newKey, oldKey); err != nil {
		t.Fatalf("expected new token to validate, got %v", err)
	}
}

func TestJwtUtil_ParseTokenWithKeys_RejectsUnknownKeys(t *testing.T) {
	token, err := utils.JwtUtil.GenerateToken(sampleClaims(), "the-real-key-xxxxxxxxxxxxxxxxxxxx")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if _, err := utils.JwtUtil.ParseTokenWithKeys(token, "other-1", "other-2"); err == nil {
		t.Fatal("expected error parsing token against unrelated keys, got nil")
	}
}

func TestJwtUtil_ParseTokenWithKeys_NoKeys(t *testing.T) {
	if _, err := utils.JwtUtil.ParseTokenWithKeys("a.b.c"); err == nil {
		t.Fatal("expected error when no keys supplied, got nil")
	}
}

func TestJwtUtil_ParseTokenWithKeys_IgnoresEmptyKeys(t *testing.T) {
	const key = "real-signing-key-cccccccccccccccccc"
	token, err := utils.JwtUtil.GenerateToken(sampleClaims(), key)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	// Empty string must not be trusted (publicly derivable Ed25519 key).
	if _, err := utils.JwtUtil.ParseTokenWithKeys(token, "", "  ", key); err != nil {
		t.Fatalf("expected real key to validate while blanks are skipped, got %v", err)
	}
	emptySigned, err := utils.JwtUtil.GenerateToken(sampleClaims(), "")
	if err != nil {
		t.Fatalf("GenerateToken(empty): %v", err)
	}
	if _, err := utils.JwtUtil.ParseTokenWithKeys(emptySigned, "", key); err == nil {
		t.Fatal("expected token signed with empty secret to be rejected when blanks are filtered")
	}
}

func TestJwtUtil_GenerateToken(t *testing.T) {
	secretKey := "test-secret-key"
	claims := jwt.RegisteredClaims{
		Subject:   "user123",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}

	token, err := utils.JwtUtil.GenerateToken(claims, secretKey)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	if token == "" {
		t.Error("Generated token is empty")
	}
}

func TestJwtUtil_ParseToken(t *testing.T) {
	secretKey := "test-secret-key"
	claims := jwt.RegisteredClaims{
		Subject:   "user123",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}

	// Generate token
	token, err := utils.JwtUtil.GenerateToken(claims, secretKey)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	// Parse token
	parsedClaims, err := utils.JwtUtil.ParseToken(token, secretKey)
	if err != nil {
		t.Fatalf("ParseToken failed: %v", err)
	}

	if parsedClaims == nil {
		t.Fatal("Parsed claims is nil")
	}

	if parsedClaims.Subject != claims.Subject {
		t.Errorf("Expected subject %s, got %s", claims.Subject, parsedClaims.Subject)
	}
}

func TestJwtUtil_ParseToken_InvalidToken(t *testing.T) {
	secretKey := "test-secret-key"
	invalidToken := "invalid.token.here"

	_, err := utils.JwtUtil.ParseToken(invalidToken, secretKey)
	if err == nil {
		t.Error("Expected error for invalid token, got nil")
	}
}

func TestJwtUtil_ParseToken_WrongKey(t *testing.T) {
	secretKey := "test-secret-key"
	wrongKey := "wrong-secret-key"

	claims := jwt.RegisteredClaims{
		Subject:   "user123",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}

	// Generate token with correct key
	token, err := utils.JwtUtil.GenerateToken(claims, secretKey)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	// Try to parse with wrong key
	_, err = utils.JwtUtil.ParseToken(token, wrongKey)
	if err == nil {
		t.Error("Expected error for wrong key, got nil")
	}
}
