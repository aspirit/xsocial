// Package notification provides notification domain types.
package notification

// NotificationEventVersion is the current schema version of NotificationPayload.
// Bump it on breaking payload changes so consumers can branch on Version.
const NotificationEventVersion = 1

// NotificationPayload is the JSON payload for notification events in message brokers.
// Used by both Kafka and Pub/Sub. ActType and EntityType use int8 for wire compatibility.
type NotificationPayload struct {
	// Version is the payload schema version (0 = legacy/pre-versioning).
	Version    int            `json:"version,omitempty"`
	ID         uint64         `json:"id"`
	UID        uint64         `json:"uid"`
	ActorID    uint64         `json:"actorId"`
	ActType    int8           `json:"actType"`
	EntityType int8           `json:"entityType"`
	EntityID   uint64         `json:"entityId"`
	Payload    map[string]any `json:"payload,omitempty"`
	CreatedAt  int64          `json:"createdAt"`
}
