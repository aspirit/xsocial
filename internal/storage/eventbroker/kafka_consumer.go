// Package eventbroker provides event broker publisher factory and Kafka consumer.
package eventbroker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"go.uber.org/zap"

	"github.com/aspirit/xsocial/internal/config"
	"github.com/aspirit/xsocial/pkg/eventbroker"
	"github.com/aspirit/xsocial/pkg/eventbroker/kafka"
	"github.com/aspirit/xsocial/pkg/logger"
)

// KafkaConsumer runs a Kafka consumer group for one or more topics.
type KafkaConsumer struct {
	consumerGroup sarama.ConsumerGroup
	topics        []string
	handler       eventbroker.Handler
	logger        *logger.Logger
	closed        bool
	mu            sync.Mutex
}

// NewKafkaConsumer creates a Kafka consumer for notification_event topic.
// Topic is fixed in events.go.
func NewKafkaConsumer(cfg config.KafkaConfig, handler eventbroker.Handler, log *logger.Logger) (*KafkaConsumer, error) {
	topics := []string{TopicNotificationEvent}

	saramaCfg := sarama.NewConfig()
	saramaCfg.Consumer.Return.Errors = true
	// Offset strategy: OffsetNewest means a brand-new consumer group starts from
	// the tail, so messages produced before the group existed are skipped
	// (intentional: no history replay on first deploy). Committed offsets are
	// always honored, so a restart resumes where it left off; delivery is
	// at-least-once and the handler is idempotent. Use OffsetOldest only when a
	// full replay is desired.
	saramaCfg.Consumer.Offsets.Initial = sarama.OffsetNewest
	saramaCfg.Consumer.Group.Rebalance.Strategy = sarama.NewBalanceStrategyRoundRobin()

	if cfg.Security.TLSEnabled {
		saramaCfg.Net.TLS.Enable = true
	}
	if kafka.IsSASLEnabled(cfg.Security.SASL.Mechanism) {
		saramaCfg.Net.SASL.Enable = true
		saramaCfg.Net.SASL.Mechanism = sarama.SASLMechanism(cfg.Security.SASL.Mechanism)
		saramaCfg.Net.SASL.User = cfg.Security.SASL.Username
		saramaCfg.Net.SASL.Password = cfg.Security.SASL.Password
	}

	consumerGroup, err := sarama.NewConsumerGroup(cfg.Brokers, SubscriptionNotificationWorker, saramaCfg)
	if err != nil {
		return nil, fmt.Errorf("create consumer group: %w", err)
	}

	return &KafkaConsumer{
		consumerGroup: consumerGroup,
		topics:        topics,
		handler:       handler,
		logger:        log.Named("eventbroker-kafka-consumer"),
	}, nil
}

// Run consumes messages until context is cancelled.
func (c *KafkaConsumer) Run(ctx context.Context) error {
	h := &consumerGroupHandler{
		handler: c.handler,
		logger:  c.logger,
	}

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("Kafka consumer shutting down")

			return nil
		default:
			if err := c.consumerGroup.Consume(ctx, c.topics, h); err != nil {
				c.logger.Warn("Consumer group session ended with error, rejoining", zap.Error(err))
				select {
				case <-ctx.Done():
				case <-time.After(consumeRejoinDelay):
				}
			}
		}
	}
}

// Close closes the consumer.
func (c *KafkaConsumer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true

	if err := c.consumerGroup.Close(); err != nil {
		return fmt.Errorf("close consumer: %w", err)
	}

	return nil
}

const (
	// handleMaxAttempts / handleRetryBase bound the in-place retry of one message
	// (500ms, 1s): enough for a MongoDB election or a timeout, short enough not
	// to stall the partition for long.
	handleMaxAttempts = 3
	handleRetryBase   = 500 * time.Millisecond
	// consumeRejoinDelay paces reconnects after a failed session.
	consumeRejoinDelay = time.Second
)

// handleWithRetry runs the handler up to attempts times with linear backoff.
// Transient failures are absorbed here; a persistent one is returned so the
// caller can stop consuming without committing past the message.
func handleWithRetry(ctx context.Context, h eventbroker.Handler, msg *eventbroker.Message, log *logger.Logger, attempts int, base time.Duration) error {
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err = h.Handle(ctx, msg); err == nil {
			return nil
		}
		if attempt == attempts {
			break
		}
		log.Warn("Failed to handle event, retrying",
			zap.String("topic", msg.Topic), zap.Int("attempt", attempt), zap.Error(err))
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w (after attempt %d: %w)", ctx.Err(), attempt, err)
		case <-time.After(base * time.Duration(attempt)):
		}
	}

	return fmt.Errorf("handler failed after %d attempts: %w", attempts, err)
}

type consumerGroupHandler struct {
	handler eventbroker.Handler
	logger  *logger.Logger
}

func (h *consumerGroupHandler) Setup(session sarama.ConsumerGroupSession) error {
	h.logger.Debug("Consumer group session started", zap.String("member_id", session.MemberID()))

	return nil
}

func (h *consumerGroupHandler) Cleanup(_ sarama.ConsumerGroupSession) error { return nil }

func (h *consumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	h.logger.Debug("ConsumeClaim started",
		zap.String("topic", claim.Topic()),
		zap.Int32("partition", claim.Partition()),
		zap.Int64("initial_offset", claim.InitialOffset()),
	)
	for msg := range claim.Messages() {
		brokerMsg := &eventbroker.Message{
			Topic:     msg.Topic,
			Key:       msg.Key,
			Value:     msg.Value,
			Timestamp: msg.Timestamp.UnixMilli(),
		}
		if len(msg.Headers) > 0 {
			brokerMsg.Headers = make(map[string]string)
			for _, hdr := range msg.Headers {
				if len(hdr.Value) > 0 {
					brokerMsg.Headers[string(hdr.Key)] = string(hdr.Value)
				}
			}
		}

		ctx := logger.WithLogger(session.Context(), h.logger)
		if err := handleWithRetry(ctx, h.handler, brokerMsg, h.logger, handleMaxAttempts, handleRetryBase); err != nil {
			// Offsets commit cumulatively per partition, so skipping this message
			// and marking a later one would silently drop it. Stop the claim
			// instead: the session ends and consumption resumes from the last
			// committed offset, i.e. this message is delivered again.
			h.logger.Error("Giving up on event, leaving the claim so the broker redelivers",
				zap.String("topic", msg.Topic),
				zap.Int32("partition", msg.Partition),
				zap.Int64("offset", msg.Offset),
				zap.Error(err),
			)

			return fmt.Errorf("handle %s/%d@%d: %w", msg.Topic, msg.Partition, msg.Offset, err)
		}

		session.MarkMessage(msg, "")
	}

	return nil
}
