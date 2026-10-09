package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	internalerr "github.com/aspirit/xsocial/internal/errors"
	"github.com/aspirit/xsocial/pkg/project"
)

func TestValidateJWTKey(t *testing.T) {
	strong := "9f3Kx7Qz-pL2mWvB8sT4rN6yH1cD5eA0uJiOgPkZ" // 40 high-entropy chars
	tests := []struct {
		name    string
		key     string
		wantErr error
	}{
		{"empty", "", internalerr.ErrJWTKeyEmpty},
		{"whitespace only", "    ", internalerr.ErrJWTKeyEmpty},
		{"too short", "short-key-123", internalerr.ErrJWTKeyTooShort},
		{"long but low entropy", strings.Repeat("ab", 32), internalerr.ErrJWTKeyLowEntropy},
		{"strong", strong, nil},
		{"openssl-like base64", "Jhk9vZ2Qw7Lr3Np6Tb8Xf1Yc4Md0Hs5Ae+Gu/IoPq", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateJWTKey(tt.key)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}

				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestValidateJWTConfig_PreviousKeys(t *testing.T) {
	strong := "Jhk9vZ2Qw7Lr3Np6Tb8Xf1Yc4Md0Hs5Ae-Gu_IoPq"
	cfg := &Config{}
	cfg.Secrets.JWT.Key = strong
	cfg.Secrets.JWT.PreviousKeys = []string{strong, "weak"}

	err := ValidateJWTConfig(cfg)
	if !errors.Is(err, internalerr.ErrJWTKeyTooShort) {
		t.Fatalf("expected previous_keys weak key to fail with ErrJWTKeyTooShort, got %v", err)
	}

	// All strong → ok.
	cfg.Secrets.JWT.PreviousKeys = []string{strong}
	if err := ValidateJWTConfig(cfg); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}

	// Empty previous key must fail validation (zero-entropy / publicly derivable).
	cfg.Secrets.JWT.PreviousKeys = []string{""}
	if err := ValidateJWTConfig(cfg); !errors.Is(err, internalerr.ErrJWTKeyEmpty) {
		t.Fatalf("expected empty previous_keys to fail with ErrJWTKeyEmpty, got %v", err)
	}
}

// TestDefaultConfigJWTKeyHasNoFallback guards the shipped default config: a
// `${JWT_KEY:-literal}` fallback would let a release deploy that forgot to set
// JWT_KEY boot and sign every token with a secret committed to this repo,
// bypassing the ValidateJWTKey fail-fast. Unset must expand to empty instead.
func TestDefaultConfigJWTKeyHasNoFallback(t *testing.T) {
	root, err := project.Root()
	if err != nil {
		t.Skipf("project root not resolvable: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "configs", "config.default.yaml"))
	if err != nil {
		t.Fatalf("read default config: %v", err)
	}

	var keyLine string
	for _, line := range strings.Split(string(data), "\n") {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "key: ${JWT_KEY") {
			keyLine = trimmed

			break
		}
	}
	if keyLine == "" {
		t.Fatal("secrets.jwt.key line not found in config.default.yaml")
	}
	if keyLine != "key: ${JWT_KEY}" {
		t.Fatalf("secrets.jwt.key must be exactly ${JWT_KEY} with no default value, got %q", keyLine)
	}
}

func TestSanitizeJWTPreviousKeys(t *testing.T) {
	strong := "Jhk9vZ2Qw7Lr3Np6Tb8Xf1Yc4Md0Hs5Ae-Gu_IoPq"
	jwt := JWTConfig{PreviousKeys: []string{"", "  ", strong, ""}}
	sanitizeJWTPreviousKeys(&jwt)
	if len(jwt.PreviousKeys) != 1 || jwt.PreviousKeys[0] != strong {
		t.Fatalf("expected only strong key retained, got %#v", jwt.PreviousKeys)
	}
}
