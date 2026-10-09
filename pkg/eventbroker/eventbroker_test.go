package eventbroker

import "testing"

func TestProviderType_IsValid(t *testing.T) {
	tests := []struct {
		name string
		p    ProviderType
		want bool
	}{
		{"kafka valid", ProviderKafka, true},
		{"pubsub valid", ProviderPubSub, true},
		{"empty invalid", ProviderType(""), false},
		{"unknown invalid", ProviderType("unknown"), false},
		{"rabbitmq invalid", ProviderType("rabbitmq"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProviderType_ProviderName(t *testing.T) {
	tests := []struct {
		name string
		p    ProviderType
		want string
	}{
		{"kafka", ProviderKafka, "kafka"},
		{"pubsub", ProviderPubSub, "pubsub"},
		{"empty", ProviderType(""), ""},
		{"custom", ProviderType("custom"), "custom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.ProviderName(); got != tt.want {
				t.Errorf("ProviderName() = %q, want %q", got, tt.want)
			}
		})
	}
}
