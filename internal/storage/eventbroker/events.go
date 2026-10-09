// Package eventbroker provides event broker publisher factory and consumers.
package eventbroker

// Topic and subscription names for event broker (Kafka/Pub/Sub).
// Add new constants when needed.
const (
	// TopicNotificationEvent: in-app notifications (e.g. "someone liked your post").
	// Consumer: Worker (NotificationHandler) → Notification record.
	TopicNotificationEvent = "notification_event"

	// SubscriptionNotificationWorker: Kafka consumer group ID and Pub/Sub subscription name for notification_event.
	SubscriptionNotificationWorker = "notification-worker"
)
