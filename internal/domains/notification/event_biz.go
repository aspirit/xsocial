// Package notification implements the business logic for notification events (Outbox).
// Uses message broker (Kafka or Pub/Sub) for async delivery instead of MongoDB.
package notification

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aspirit/xsocial/internal/model"
	storageeventbroker "github.com/aspirit/xsocial/internal/storage/eventbroker"
	pkgeventbroker "github.com/aspirit/xsocial/pkg/eventbroker"
	"github.com/aspirit/xsocial/pkg/idgen"
	"github.com/aspirit/xsocial/pkg/utils"
)

// INotificationEventBiz defines the interface for notification event business logic.
type INotificationEventBiz interface {
	// EnqueueNotificationEvent enqueues a notification event for async processing.
	// uid: recipient (user to notify), actorID: triggering user
	EnqueueNotificationEvent(ctx context.Context, uid, actorID uint64, actType model.ActionTypes, entityType model.NotificationEventEntityType, entityID uint64, payload map[string]any) error
}

// NotificationEventBiz implements INotificationEventBiz using a message broker.
type NotificationEventBiz struct {
	publisher pkgeventbroker.Publisher
	idGen     idgen.Generator
}

// NewNotificationEventBiz creates a new notification event business logic instance.
// Uses eventbroker.Publisher (Kafka or Pub/Sub) for event delivery.
func NewNotificationEventBiz(publisher pkgeventbroker.Publisher, idGen idgen.Generator) INotificationEventBiz {
	return &NotificationEventBiz{
		publisher: publisher,
		idGen:     idGen,
	}
}

// EnqueueNotificationEvent publishes a notification event to the message broker.
func (b *NotificationEventBiz) EnqueueNotificationEvent(ctx context.Context, uid, actorID uint64, actType model.ActionTypes, entityType model.NotificationEventEntityType, entityID uint64, payload map[string]any) error {
	id, err := b.idGen.NextID()
	if err != nil {
		return fmt.Errorf("failed to generate notification event ID: %w", err)
	}

	np := &NotificationPayload{
		Version:    NotificationEventVersion,
		ID:         id,
		UID:        uid,
		ActorID:    actorID,
		ActType:    int8(actType),
		EntityType: int8(entityType),
		EntityID:   entityID,
		Payload:    payload,
		CreatedAt:  utils.TimeUtil.Now(),
	}

	value, err := json.Marshal(np)
	if err != nil {
		return fmt.Errorf("marshal notification payload: %w", err)
	}

	msg := &pkgeventbroker.Message{
		Topic: storageeventbroker.TopicNotificationEvent,
		Key:   []byte(fmt.Sprintf("%d", id)),
		Value: value,
		Headers: map[string]string{
			"event_id": fmt.Sprintf("%d", id),
		},
		Timestamp: np.CreatedAt,
	}

	if err := b.publisher.Publish(ctx, msg); err != nil {
		return fmt.Errorf("failed to enqueue notification event: %w", err)
	}

	return nil
}
