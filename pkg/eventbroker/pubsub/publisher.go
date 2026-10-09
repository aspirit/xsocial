// Package pubsub provides GCP Pub/Sub implementation for event broker publishing.
package pubsub

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"go.uber.org/zap"

	"github.com/aspirit/xsocial/pkg/eventbroker"
	"github.com/aspirit/xsocial/pkg/logger"
)

// ErrProjectIDRequired is returned when no GCP project ID is configured.
var ErrProjectIDRequired = errors.New("pubsub: project ID is required")

// Config holds Pub/Sub publisher configuration.
type Config struct {
	ProjectID string
	// DefaultTopic is optional; Message.Topic takes precedence.
	DefaultTopic string
}

// Publisher implements eventbroker.Publisher using GCP Pub/Sub.
type Publisher struct {
	client     *pubsub.Client
	projectID  string
	publishers sync.Map // map[string]*pubsub.Publisher
	logger     *logger.Logger
}

// NewPublisher creates a Pub/Sub publisher for generic event messaging.
func NewPublisher(cfg Config, log *logger.Logger) (eventbroker.Publisher, error) {
	projectID := cfg.ProjectID
	if projectID == "" {
		return nil, fmt.Errorf("new publisher: %w", ErrProjectIDRequired)
	}

	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create pubsub client: %w", err)
	}

	return &Publisher{
		client:    client,
		projectID: projectID,
		logger:    log.Named("eventbroker-pubsub"),
	}, nil
}

// getPublisher returns the Pub/Sub publisher for the given topic, creating if needed.
func (p *Publisher) getPublisher(ctx context.Context, topicName string) (*pubsub.Publisher, error) {
	if v, ok := p.publishers.Load(topicName); ok {
		return v.(*pubsub.Publisher), nil
	}

	topicPath := fmt.Sprintf("projects/%s/topics/%s", p.projectID, topicName)
	if _, err := p.client.TopicAdminClient.GetTopic(ctx, &pubsubpb.GetTopicRequest{Topic: topicPath}); err != nil {
		return nil, fmt.Errorf("pubsub topic %q does not exist: %w", topicName, err)
	}

	pub := p.client.Publisher(topicName)
	p.publishers.Store(topicName, pub)

	return pub, nil
}

// Publish sends a message to Pub/Sub. Topic is taken from msg.Topic.
func (p *Publisher) Publish(ctx context.Context, msg *eventbroker.Message) error {
	topic := msg.Topic
	if topic == "" {
		return eventbroker.ErrMessageTopicRequired
	}

	pub, err := p.getPublisher(ctx, topic)
	if err != nil {
		return err
	}

	psMsg := &pubsub.Message{
		Data: msg.Value,
	}
	if msg.Headers != nil {
		psMsg.Attributes = msg.Headers
	}

	result := pub.Publish(ctx, psMsg)
	if _, err := result.Get(ctx); err != nil {
		p.logger.Warn("Failed to publish message", zap.String("topic", topic), zap.Error(err))

		return fmt.Errorf("publish: %w", err)
	}

	p.logger.Debug("Published message", zap.String("topic", topic))

	return nil
}

// Close closes the Pub/Sub client.
func (p *Publisher) Close() error {
	p.publishers.Range(func(_, v any) bool {
		v.(*pubsub.Publisher).Stop()

		return true
	})

	if err := p.client.Close(); err != nil {
		return fmt.Errorf("close pubsub client: %w", err)
	}

	return nil
}

var _ eventbroker.Publisher = (*Publisher)(nil)
