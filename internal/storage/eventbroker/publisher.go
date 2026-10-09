// Package eventbroker provides event broker publisher factory and consumers.
package eventbroker

import (
	"fmt"
	"strings"

	"github.com/aspirit/xsocial/internal/config"
	internalerr "github.com/aspirit/xsocial/internal/errors"
	pkgeventbroker "github.com/aspirit/xsocial/pkg/eventbroker"
	"github.com/aspirit/xsocial/pkg/eventbroker/kafka"
	"github.com/aspirit/xsocial/pkg/eventbroker/pubsub"
	"github.com/aspirit/xsocial/pkg/logger"
)

// NewEventBrokerPublisher creates a generic Publisher based on notification_broker.type config.
// Publisher supports multiple topics via Message.Topic.
func NewEventBrokerPublisher(cfg *config.Config, log *logger.Logger) (pkgeventbroker.Publisher, error) {
	brokerType := strings.ToLower(strings.TrimSpace(cfg.Storage.NotificationBroker.Type))
	if brokerType == "" {
		brokerType = string(pkgeventbroker.ProviderKafka)
	}

	switch brokerType {
	case string(pkgeventbroker.ProviderKafka):
		if len(cfg.Storage.Kafka.Brokers) == 0 {
			return nil, internalerr.ErrKafkaBrokersNotConfigured
		}

		kafkaCfg := kafka.Config{
			Brokers: cfg.Storage.Kafka.Brokers,
			Security: kafka.SecurityConfig{
				TLSEnabled: cfg.Storage.Kafka.Security.TLSEnabled,
				SASL: kafka.SASLConfig{
					Mechanism: cfg.Storage.Kafka.Security.SASL.Mechanism,
					Username:  cfg.Storage.Kafka.Security.SASL.Username,
					Password:  cfg.Storage.Kafka.Security.SASL.Password,
				},
			},
		}

		pub, err := kafka.NewPublisher(kafkaCfg, log)
		if err != nil {
			return nil, fmt.Errorf("kafka publisher: %w", err)
		}

		return pub, nil
	case string(pkgeventbroker.ProviderPubSub):
		if cfg.Storage.NotificationBroker.PubSubProjectID == "" {
			return nil, internalerr.ErrPubSubProjectIDRequired
		}
		pubsubCfg := pubsub.Config{
			ProjectID: cfg.Storage.NotificationBroker.PubSubProjectID,
		}

		pub, err := pubsub.NewPublisher(pubsubCfg, log)
		if err != nil {
			return nil, fmt.Errorf("pubsub publisher: %w", err)
		}

		return pub, nil
	default:
		return nil, fmt.Errorf("%w: %q (use %s or %s)", internalerr.ErrUnsupportedNotificationBrokerType, brokerType, pkgeventbroker.ProviderKafka, pkgeventbroker.ProviderPubSub)
	}
}
