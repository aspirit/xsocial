// Package errors provides system-level error definitions.
package errors

import "errors"

// System and infrastructure errors.
var (
	// Event broker.
	ErrKafkaBrokersNotConfigured         = errors.New("kafka brokers not configured (KAFKA_BROKERS)")
	ErrUnsupportedNotificationBrokerType = errors.New("unsupported notification_broker.type (use kafka or pubsub)")
	ErrPubSubProjectIDRequired           = errors.New("pubsub requires project_id (GCP_PROJECT_ID)")
	ErrUnexpectedNotificationTopic       = errors.New("unexpected topic for notification handler")

	// Service configuration.
	ErrServiceIDRequired      = errors.New("service ID required")
	ErrServiceIDInvalidPrefix = errors.New("service ID invalid prefix")
	ErrServiceAddressRequired = errors.New("service address required")

	// General.
	ErrNotImplemented = errors.New("not implemented")
	ErrInternalError  = errors.New("internal server error")
)
