// Package eventbroker provides event broker publisher factory and Pub/Sub Pull consumer.
package eventbroker

import (
	"context"
	"fmt"
	"sync"

	"cloud.google.com/go/pubsub/v2"
	"go.uber.org/zap"

	"github.com/aspirit/xsocial/internal/config"
	internalerr "github.com/aspirit/xsocial/internal/errors"
	"github.com/aspirit/xsocial/pkg/eventbroker"
	"github.com/aspirit/xsocial/pkg/logger"
)

// PubSubPullConsumer runs a Pub/Sub Pull subscription consumer.
// Worker pulls messages from GCP; no HTTP endpoint needed.
type PubSubPullConsumer struct {
	client  *pubsub.Client
	sub     *pubsub.Subscriber
	handler eventbroker.Handler
	logger  *logger.Logger
	closed  bool
	mu      sync.Mutex
}

// NewPubSubPullConsumer creates a Pub/Sub Pull consumer for notification_event.
// Subscription must exist in GCP (e.g. gcloud pubsub subscriptions create notification-worker --topic=notification_event).
func NewPubSubPullConsumer(cfg config.NotificationBrokerConfig, handler eventbroker.Handler, log *logger.Logger) (*PubSubPullConsumer, error) {
	projectID := cfg.PubSubProjectID
	if projectID == "" {
		return nil, internalerr.ErrPubSubProjectIDRequired
	}

	subscriptionID := cfg.PubSubSubscription
	if subscriptionID == "" {
		subscriptionID = SubscriptionNotificationWorker
	}

	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create pubsub client: %w", err)
	}

	sub := client.Subscriber(subscriptionID)

	return &PubSubPullConsumer{
		client:  client,
		sub:     sub,
		handler: handler,
		logger:  log.Named("eventbroker-pubsub-pull"),
	}, nil
}

// Run consumes messages until context is cancelled.
func (c *PubSubPullConsumer) Run(ctx context.Context) error {
	c.logger.Info("Pub/Sub Pull consumer starting",
		zap.String("subscription", c.sub.String()),
	)

	if err := c.sub.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		topic := TopicNotificationEvent
		if msg.Attributes != nil {
			if t := msg.Attributes["topic"]; t != "" {
				topic = t
			}
		}

		brokerMsg := &eventbroker.Message{
			Topic:   topic,
			Value:   msg.Data,
			Headers: msg.Attributes,
		}

		ctx = logger.WithLogger(ctx, c.logger)
		if err := c.handler.Handle(ctx, brokerMsg); err != nil {
			c.logger.Warn("Failed to handle event",
				zap.String("topic", topic),
				zap.Error(err),
			)
			msg.Nack()

			return
		}

		msg.Ack()
	}); err != nil {
		return fmt.Errorf("pubsub receive: %w", err)
	}

	return nil
}

// Close closes the consumer and Pub/Sub client.
func (c *PubSubPullConsumer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true

	if err := c.client.Close(); err != nil {
		return fmt.Errorf("close pubsub client: %w", err)
	}

	return nil
}
