package kafka

import "testing"

func TestIsSASLEnabled(t *testing.T) {
	tests := []struct {
		name      string
		mechanism string
		want      bool
	}{
		{"OAUTHBEARER", "OAUTHBEARER", true},
		{"PLAIN", "PLAIN", true},
		{"SCRAM-SHA-256", "SCRAM-SHA-256", true},
		{"SCRAM-SHA-512", "SCRAM-SHA-512", true},
		{"GSSAPI", "GSSAPI", true},
		{"empty", "", false},
		{"none", "none", false},
		{"None", "None", false},
		{"unknown", "unknown", false},
		{"plain lowercase", "plain", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSASLEnabled(tt.mechanism); got != tt.want {
				t.Errorf("IsSASLEnabled(%q) = %v, want %v", tt.mechanism, got, tt.want)
			}
		})
	}
}
