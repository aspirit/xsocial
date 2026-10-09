// Package kafka provides Kafka implementation for event broker publishing.
package kafka

import (
	"context"
	"fmt"

	"github.com/IBM/sarama"
	"go.uber.org/zap"

	"github.com/aspirit/xsocial/pkg/eventbroker"
	"github.com/aspirit/xsocial/pkg/logger"
)

// Config holds Kafka publisher configuration.
type Config struct {
	Brokers  []string
	Security SecurityConfig
}

// SecurityConfig holds Kafka security settings.
type SecurityConfig struct {
	TLSEnabled bool
	SASL       SASLConfig
}

// SASLConfig holds SASL authentication settings.
type SASLConfig struct {
	Mechanism string
	Username  string
	Password  string
}

// IsSASLEnabled returns true only for valid Sarama SASL mechanisms.
// Empty, "none", or invalid values mean PLAINTEXT (no auth).
func IsSASLEnabled(mechanism string) bool {
	switch mechanism {
	case "OAUTHBEARER", "PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512", "GSSAPI":
		return true
	default:
		return false
	}
}

// Publisher implements eventbroker.Publisher using Kafka.
type Publisher struct {
	producer sarama.SyncProducer
	logger   *logger.Logger
}

// NewPublisher creates a Kafka publisher for generic event messaging.
func NewPublisher(cfg Config, log *logger.Logger) (eventbroker.Publisher, error) {
	saramaCfg := sarama.NewConfig()
	saramaCfg.Producer.RequiredAcks = sarama.WaitForLocal
	saramaCfg.Producer.Return.Successes = true

	if cfg.Security.TLSEnabled {
		saramaCfg.Net.TLS.Enable = true
	}
	if IsSASLEnabled(cfg.Security.SASL.Mechanism) {
		saramaCfg.Net.SASL.Enable = true
		saramaCfg.Net.SASL.Mechanism = sarama.SASLMechanism(cfg.Security.SASL.Mechanism)
		saramaCfg.Net.SASL.User = cfg.Security.SASL.Username
		saramaCfg.Net.SASL.Password = cfg.Security.SASL.Password
	}

	producer, err := sarama.NewSyncProducer(cfg.Brokers, saramaCfg)
	if err != nil {
		return nil, fmt.Errorf("create kafka producer: %w", err)
	}

	return &Publisher{
		producer: producer,
		logger:   log.Named("eventbroker-kafka"),
	}, nil
}

// Publish sends a message to Kafka. Topic is taken from msg.Topic.
func (p *Publisher) Publish(ctx context.Context, msg *eventbroker.Message) error {
	topic := msg.Topic
	if topic == "" {
		return eventbroker.ErrMessageTopicRequired
	}

	key := msg.Key
	if key == nil {
		key = []byte{}
	}

	saramaMsg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.ByteEncoder(key),
		Value: sarama.ByteEncoder(msg.Value),
	}

	_, _, err := p.producer.SendMessage(saramaMsg)
	if err != nil {
		p.logger.Warn("Failed to publish message", zap.String("topic", topic), zap.Error(err))

		return fmt.Errorf("send message: %w", err)
	}

	p.logger.Debug("Published message", zap.String("topic", topic))

	return nil
}

// Close closes the Kafka producer.
func (p *Publisher) Close() error {
	if err := p.producer.Close(); err != nil {
		return fmt.Errorf("close producer: %w", err)
	}

	return nil
}

var _ eventbroker.Publisher = (*Publisher)(nil)
