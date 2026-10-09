package config

import (
	"errors"
	"testing"

	internalerr "github.com/aspirit/xsocial/internal/errors"
)

func TestValidateRateLimitEnabled(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		enabled bool
		wantErr bool
	}{
		{"release disabled fails", ModeRelease, false, true},
		{"release enabled ok", ModeRelease, true, false},
		{"debug disabled ok", ModeDebug, false, false},
		{"debug enabled ok", ModeDebug, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			cfg.Project.Mode = tt.mode
			cfg.RateLimit.Enabled = tt.enabled
			err := ValidateRateLimitEnabled(cfg)
			if tt.wantErr {
				if !errors.Is(err, internalerr.ErrRateLimitDisabledRelease) {
					t.Fatalf("expected ErrRateLimitDisabledRelease, got %v", err)
				}

				return
			}
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
