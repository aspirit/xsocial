package config

import "testing"

func TestBusinessValidation_IsUsername(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid 5 chars", "abc12", true},
		{"valid 10 chars", "abc123def", true},
		{"valid 20 chars", "a1b2c3d4e5f6g7h8i9j0", true},
		{"with underscore", "user_name_123", true},
		{"uppercase start", "UserName123", true},
		{"too short 3", "abc", false},
		{"too short 4", "ab12", false},
		{"too long 21", "a1b2c3d4e5f6g7h8i9j0k", false},
		{"starts with number", "12345", false},
		{"starts with underscore", "_user", false},
		{"contains space", "user name", false},
		{"contains dash", "user-name", false},
		{"contains dot", "user.name", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BusinessValidator.IsUsername(tt.input)
			if result != tt.expected {
				t.Errorf("IsUsername(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNewBusinessValidation(t *testing.T) {
	validator := NewBusinessValidation()
	if validator == nil {
		t.Fatal("NewBusinessValidation() returned nil")
	}

	if validator.usernameRegex == nil {
		t.Error("usernameRegex should not be nil")
	}
}
