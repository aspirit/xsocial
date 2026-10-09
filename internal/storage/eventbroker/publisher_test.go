package eventbroker

import (
	"errors"
	"testing"

	"github.com/aspirit/xsocial/internal/config"
	internalerr "github.com/aspirit/xsocial/internal/errors"
	"github.com/aspirit/xsocial/pkg/logger"
)

func TestNewEventBrokerPublisher(t *testing.T) {
	log := logger.NewNopLogger()

	t.Run("kafka_empty_brokers", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Storage.NotificationBroker.Type = "kafka"
		cfg.Storage.Kafka.Brokers = nil

		_, err := NewEventBrokerPublisher(cfg, log)
		if !errors.Is(err, internalerr.ErrKafkaBrokersNotConfigured) {
			t.Errorf("NewEventBrokerPublisher() err = %v, want ErrKafkaBrokersNotConfigured", err)
		}
	})

	t.Run("kafka_empty_brokers_slice", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Storage.NotificationBroker.Type = "kafka"
		cfg.Storage.Kafka.Brokers = []string{}

		_, err := NewEventBrokerPublisher(cfg, log)
		if !errors.Is(err, internalerr.ErrKafkaBrokersNotConfigured) {
			t.Errorf("NewEventBrokerPublisher() err = %v, want ErrKafkaBrokersNotConfigured", err)
		}
	})

	t.Run("unknown_type", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Storage.NotificationBroker.Type = "rabbitmq"

		_, err := NewEventBrokerPublisher(cfg, log)
		if !errors.Is(err, internalerr.ErrUnsupportedNotificationBrokerType) {
			t.Errorf("NewEventBrokerPublisher() err = %v, want ErrUnsupportedNotificationBrokerType", err)
		}
	})

	t.Run("pubsub_empty_project", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Storage.NotificationBroker.Type = "pubsub"
		cfg.Storage.NotificationBroker.PubSubProjectID = ""

		_, err := NewEventBrokerPublisher(cfg, log)
		if !errors.Is(err, internalerr.ErrPubSubProjectIDRequired) {
			t.Errorf("NewEventBrokerPublisher() err = %v, want ErrPubSubProjectIDRequired", err)
		}
	})

	t.Run("empty_type_defaults_to_kafka", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Storage.NotificationBroker.Type = ""
		cfg.Storage.Kafka.Brokers = nil

		_, err := NewEventBrokerPublisher(cfg, log)
		if !errors.Is(err, internalerr.ErrKafkaBrokersNotConfigured) {
			t.Errorf("NewEventBrokerPublisher() err = %v, want ErrKafkaBrokersNotConfigured (empty type defaults to kafka)", err)
		}
	})
}
