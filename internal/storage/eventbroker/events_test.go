package eventbroker

import "testing"

func TestTopicConstants(t *testing.T) {
	// Ensure topic constants are non-empty and unique.
	topics := map[string]string{
		"TopicNotificationEvent": TopicNotificationEvent,
	}

	seen := make(map[string]string)
	for name, val := range topics {
		if val == "" {
			t.Errorf("%s is empty", name)
		}
		if prev, ok := seen[val]; ok {
			t.Errorf("duplicate topic %q: %s and %s", val, prev, name)
		}
		seen[val] = name
	}
}
