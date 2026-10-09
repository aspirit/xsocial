package utils

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// ErrNoSigningKey is returned when no signing key is supplied for parsing.
var ErrNoSigningKey = errors.New("no JWT signing key supplied")

var JwtUtil = &jwtUtil{}

type jwtUtil struct{}

// KidForKey derives a stable, non-secret key identifier from a signing secret.
// It never exposes the secret. Rotating the secret changes the kid, which lets
// parsers select the correct key during a rotation window.
func KidForKey(secret string) string {
	sum := sha256.Sum256([]byte("xs-jwt-kid\x00" + secret))

	return hex.EncodeToString(sum[:])[:16]
}

// GenerateToken signs claims with the EdDSA key derived from key and stamps a
// kid header identifying that key.
func (util *jwtUtil) GenerateToken(claims jwt.RegisteredClaims, key string) (string, error) {
	privateKey := deriveEd25519PrivateKey(key)
	tokenClaims := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	tokenClaims.Header["kid"] = KidForKey(key)
	token, err := tokenClaims.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to generate JWT token: %w", err)
	}

	return token, nil
}

// ParseToken verifies a token against a single key. Backward-compatible entry point.
func (util *jwtUtil) ParseToken(token, key string) (*jwt.RegisteredClaims, error) {
	return util.ParseTokenWithKeys(token, key)
}

// ParseTokenWithKeys verifies a token against any of the provided keys and is the
// groundwork for zero-downtime key rotation: pass the current key plus any
// previous keys whose tokens may still be in flight.
//
// If the token carries a kid matching one of the keys, that key is tried first;
// otherwise every key is tried in order (so legacy tokens minted before kid
// support, which have no kid header, still validate against the current key).
func (util *jwtUtil) ParseTokenWithKeys(token string, keys ...string) (*jwt.RegisteredClaims, error) {
	keys = filterNonEmptySigningKeys(keys)
	if len(keys) == 0 {
		return nil, fmt.Errorf("parse token: %w", ErrNoSigningKey)
	}
	var firstErr error
	for _, key := range orderKeysByTokenKid(token, keys) {
		var claims jwt.RegisteredClaims
		publicKey := deriveEd25519PrivateKey(key).Public().(ed25519.PublicKey)
		_, err := jwt.ParseWithClaims(token, &claims, func(*jwt.Token) (any, error) {
			return publicKey, nil
		}, jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}))
		if err == nil {
			return &claims, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}

	return nil, fmt.Errorf("failed to parse JWT token: %w", firstErr)
}

// filterNonEmptySigningKeys drops blank secrets. sha256("") yields a publicly
// known Ed25519 key; accepting it would let anyone forge tokens.
func filterNonEmptySigningKeys(keys []string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if strings.TrimSpace(k) == "" {
			continue
		}
		out = append(out, k)
	}

	return out
}

// orderKeysByTokenKid returns keys with the one whose kid matches the token's
// kid header moved to the front (so the expected key is tried first and its
// error — e.g. "expired" — is surfaced over an unrelated "signature invalid").
func orderKeysByTokenKid(token string, keys []string) []string {
	kid := tokenKid(token)
	if kid == "" {
		return keys
	}
	for i, key := range keys {
		if KidForKey(key) != kid {
			continue
		}
		if i == 0 {
			return keys
		}
		ordered := make([]string, 0, len(keys))
		ordered = append(ordered, key)
		ordered = append(ordered, keys[:i]...)
		ordered = append(ordered, keys[i+1:]...)

		return ordered
	}

	return keys
}

// tokenKid best-effort decodes the kid from a JWT header without verifying it.
func tokenKid(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[0], "="))
	if err != nil {
		return ""
	}
	var header struct {
		Kid string `json:"kid"`
	}
	if json.Unmarshal(headerBytes, &header) != nil {
		return ""
	}

	return header.Kid
}

// deriveEd25519PrivateKey deterministically derives an Ed25519 private key from configured JWT secret.
// This keeps configuration simple while enforcing EdDSA signing and verification.
// The signing strength therefore depends entirely on the secret's entropy; see
// config.ValidateJWTKey, which fail-fasts on weak keys in release mode.
func deriveEd25519PrivateKey(secret string) ed25519.PrivateKey {
	seed := sha256.Sum256([]byte(secret))

	return ed25519.NewKeyFromSeed(seed[:])
}
