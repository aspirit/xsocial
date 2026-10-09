// Package eventbroker provides generic event messaging interfaces (X.com-style platform).
// Supports Kafka (local) and GCP Pub/Sub (production) via provider abstraction.
// Multiple topics and payload types; each domain defines its own payload and Handler.
package eventbroker

import (
	"context"
	"errors"
)

// ErrMessageTopicRequired is returned when Message.Topic is empty.
var ErrMessageTopicRequired = errors.New("message topic is required")

// Message is the generic envelope for broker messages. Domain-specific payloads are in Value.
type Message struct {
	Topic     string
	Key       []byte
	Value     []byte
	Headers   map[string]string
	Timestamp int64
}

// Publisher publishes messages to a broker. Topic is per-message for multi-topic support.
type Publisher interface {
	Publish(ctx context.Context, msg *Message) error
	Close() error
}

// Handler processes a single message. Implementations deserialize Value based on Topic.
type Handler interface {
	Handle(ctx context.Context, msg *Message) error
}

// ProviderType represents the event broker provider type.
type ProviderType string

const (
	ProviderKafka  ProviderType = "kafka"
	ProviderPubSub ProviderType = "pubsub"
)

// ProviderName returns the string representation of the provider type.
func (p ProviderType) ProviderName() string {
	return string(p)
}

// IsValid checks if the provider type is valid.
func (p ProviderType) IsValid() bool {
	switch p {
	case ProviderKafka, ProviderPubSub:
		return true
	default:
		return false
	}
}
