package utils

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TestParseTokenWithKeys_LegacyTokenWithoutKid ensures tokens minted before kid
// support (no kid header) still validate against the current key, so existing
// sessions survive the upgrade.
func TestParseTokenWithKeys_LegacyTokenWithoutKid(t *testing.T) {
	const key = "legacy-key-without-kid-header-xx"
	priv := deriveEd25519PrivateKey(key)
	claims := jwt.RegisteredClaims{
		Subject:   "legacy-user",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	// Deliberately do NOT set a kid header (simulating a pre-rotation token).
	legacy, err := tok.SignedString(priv)
	if err != nil {
		t.Fatalf("sign legacy token: %v", err)
	}
	if _, ok := tok.Header["kid"]; ok {
		t.Fatal("test setup error: legacy token unexpectedly has kid")
	}

	parsed, err := JwtUtil.ParseTokenWithKeys(legacy, key)
	if err != nil {
		t.Fatalf("expected legacy kid-less token to parse, got %v", err)
	}
	if parsed.Subject != "legacy-user" {
		t.Fatalf("subject mismatch: %q", parsed.Subject)
	}
}
