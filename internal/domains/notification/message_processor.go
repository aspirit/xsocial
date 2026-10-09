// Package notification provides the Handler for notification events from the message broker.
package notification

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	internalerr "github.com/aspirit/xsocial/internal/errors"
	"github.com/aspirit/xsocial/internal/model"
	storageeventbroker "github.com/aspirit/xsocial/internal/storage/eventbroker"
	pkgeventbroker "github.com/aspirit/xsocial/pkg/eventbroker"
	"github.com/aspirit/xsocial/pkg/logger"
)

// NotificationHandler implements eventbroker.Handler for the notification_event
// topic. It is safe to call for the same message any number of times: the merge
// is keyed by event id, so at-least-once delivery never double counts.
type NotificationHandler struct {
	notificationBiz INotificationBiz
}

// NewNotificationHandler creates a handler for notification_event messages.
func NewNotificationHandler(notificationBiz INotificationBiz) *NotificationHandler {
	return &NotificationHandler{notificationBiz: notificationBiz}
}

// Handle processes one message carrying a NotificationPayload.
func (h *NotificationHandler) Handle(ctx context.Context, msg *pkgeventbroker.Message) error {
	if msg.Topic != storageeventbroker.TopicNotificationEvent {
		return fmt.Errorf("%w: %q", internalerr.ErrUnexpectedNotificationTopic, msg.Topic)
	}

	var payload NotificationPayload
	if err := json.Unmarshal(msg.Value, &payload); err != nil {
		return fmt.Errorf("unmarshal notification payload: %w", err)
	}

	ev := model.NotificationEvent{
		UID:      payload.UID,
		ActorID:  payload.ActorID,
		ActType:  payload.ActType,
		PostType: model.PostTypePost,
		PostID:   notificationTargetID(&payload),
		EventID:  payload.ID,
	}
	if err := h.notificationBiz.CreateOrMergeNotification(ctx, ev); err != nil {
		return fmt.Errorf("create or merge notification: %w", err)
	}

	if log := logger.LoggerFromContext(ctx); log != nil {
		log.Debug("Applied notification event",
			zap.Uint64("event_id", payload.ID),
			zap.Uint64("uid", payload.UID),
			zap.Uint64("actor_id", payload.ActorID),
			zap.Int8("act_type", payload.ActType),
		)
	}

	return nil
}

var _ pkgeventbroker.Handler = (*NotificationHandler)(nil)

// notificationTargetID derives the merge target of an event: post events merge
// on the post they concern, user-level events on the recipient (post id 0).
func notificationTargetID(p *NotificationPayload) uint64 {
	if model.NotificationEventEntityType(p.EntityType) == model.NotificationEventEntityUser {
		return 0
	}

	return p.EntityID
}
