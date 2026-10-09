// Package notification implements the inbox: one entry per (recipient, action,
// target) that absorbs events until the recipient reads it.
package notification

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	internalerr "github.com/aspirit/xsocial/internal/errors"
	"github.com/aspirit/xsocial/internal/model"
	"github.com/aspirit/xsocial/internal/storage/interfaces"
	"github.com/aspirit/xsocial/pkg/idgen"
	"github.com/aspirit/xsocial/pkg/otelutil"
)

const (
	// DefaultWhoMaxSize caps the actors kept for display; who_count still counts all.
	DefaultWhoMaxSize = 10
	// DefaultEventIDsMax bounds the event ids remembered per entry (the
	// idempotency window). A redelivery older than that many newer events on the
	// same entry would be counted again; brokers redeliver within seconds.
	DefaultEventIDsMax = 1000
)

// MergeConfig tunes how events merge into an entry.
type MergeConfig struct {
	WhoMaxSize  int
	EventIDsMax int
}

// INotificationBiz defines the business logic interface for the Notification domain.
type INotificationBiz interface {
	// CreateOrMergeNotification applies one event: it merges into the recipient's
	// unread entry for the target or creates that entry. Safe to call any number
	// of times for the same event id (at-least-once delivery).
	CreateOrMergeNotification(ctx context.Context, ev model.NotificationEvent) error
	GetNotificationsByUserID(ctx context.Context, userID uint64, limit, offset int) (*NotificationCollection, error)
	MarkAsRead(ctx context.Context, userID uint64, id string) error
	MarkAllAsRead(ctx context.Context, userID uint64) error
	GetUnreadCount(ctx context.Context, userID uint64) (int64, error)
}

// NotificationBiz implements INotificationBiz.
type NotificationBiz struct {
	notificationRepo interfaces.INotificationRepository
	idGen            idgen.Generator
	mergeConfig      MergeConfig
}

// NewNotificationBiz creates the notification domain service. mergeConfig may
// be nil to use the defaults.
func NewNotificationBiz(
	notificationRepo interfaces.INotificationRepository,
	idGen idgen.Generator,
	mergeConfig *MergeConfig,
) INotificationBiz {
	cfg := MergeConfig{WhoMaxSize: DefaultWhoMaxSize, EventIDsMax: DefaultEventIDsMax}
	if mergeConfig != nil {
		if mergeConfig.WhoMaxSize > 0 {
			cfg.WhoMaxSize = mergeConfig.WhoMaxSize
		}
		if mergeConfig.EventIDsMax > 0 {
			cfg.EventIDsMax = mergeConfig.EventIDsMax
		}
	}

	return &NotificationBiz{
		notificationRepo: notificationRepo,
		idGen:            idGen,
		mergeConfig:      cfg,
	}
}

// CreateOrMergeNotification applies ev exactly once. There is no lookup to
// race against: the database refuses, atomically, any write that would record
// the event id a second time (unique index over (uid, event_ids), spanning read
// and unread entries), and any second unread entry for the target.
//
//  1. Merge into the unread entry. "Already applied" (on this entry or on one
//     the recipient has read since) ends here.
//  2. No unread entry: create one carrying the event. Rejected because the id is
//     known → done. Rejected because another worker created the entry first →
//     merge into it (one retry is enough: the entry now exists).
func (b *NotificationBiz) CreateOrMergeNotification(ctx context.Context, ev model.NotificationEvent) error {
	for range 2 {
		res, err := b.notificationRepo.MergeEvent(ctx, ev, b.mergeConfig.WhoMaxSize, b.mergeConfig.EventIDsMax)
		if err != nil {
			return fmt.Errorf("merge notification event: %w", err)
		}
		switch res {
		case interfaces.MergeApplied:
			otelutil.Count(ctx, "notification_merged_total", 1)

			return nil
		case interfaces.MergeAlreadyApplied:
			otelutil.Count(ctx, "notification_duplicate_total", 1)

			return nil
		case interfaces.MergeNoUnreadEntry:
		}

		id, err := b.idGen.NextID()
		if err != nil {
			return fmt.Errorf("generate notification id: %w", err)
		}
		err = b.notificationRepo.Create(ctx, model.NewNotification(id, ev))
		switch {
		case err == nil:
			return nil
		case errors.Is(err, internalerr.ErrDuplicateEvent):
			otelutil.Count(ctx, "notification_duplicate_total", 1)

			return nil
		case errors.Is(err, internalerr.ErrDuplicateRecord):
			continue // lost the create race; the unread entry exists now
		default:
			return fmt.Errorf("create notification: %w", err)
		}
	}

	return fmt.Errorf("%w: notification for uid %d lost the create race twice", internalerr.ErrInternalError, ev.UID)
}

func (b *NotificationBiz) GetNotificationsByUserID(ctx context.Context, userID uint64, limit, offset int) (*NotificationCollection, error) {
	notifications, err := b.notificationRepo.FindByUIDWithOffset(ctx, userID, int64(limit), int64(offset))
	if err != nil {
		return nil, fmt.Errorf("failed to find notifications by user id: %w", err)
	}

	unreadCount, err := b.notificationRepo.CountUnreadByUID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to count unread notifications: %w", err)
	}

	notificationObjects := make([]NotificationObject, 0, len(notifications))
	for _, notification := range notifications {
		notificationObjects = append(notificationObjects, toObject(&notification))
	}

	total, _ := b.notificationRepo.CountByUID(ctx, userID)

	return &NotificationCollection{
		Notifications: notificationObjects,
		Total:         total,
		UnreadCount:   unreadCount,
	}, nil
}

// MarkAsRead marks one of userID's notifications as read. The ownership check is
// part of the update filter, so a user can never mark someone else's notification.
func (b *NotificationBiz) MarkAsRead(ctx context.Context, userID uint64, id string) error {
	idUint64, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return internalerr.ErrInvalidParams
	}
	found, err := b.notificationRepo.MarkAsReadForUser(ctx, userID, idUint64)
	if err != nil {
		return fmt.Errorf("failed to mark notification as read: %w", err)
	}
	if !found {
		return internalerr.ErrRecordNotFound
	}

	return nil
}

func (b *NotificationBiz) MarkAllAsRead(ctx context.Context, userID uint64) error {
	err := b.notificationRepo.MarkAllAsReadByUID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to mark all notifications as read: %w", err)
	}

	return nil
}

func (b *NotificationBiz) GetUnreadCount(ctx context.Context, userID uint64) (int64, error) {
	count, err := b.notificationRepo.CountUnreadByUID(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to get unread count: %w", err)
	}

	return count, nil
}

func toObject(n *model.Notification) NotificationObject {
	who := make([]string, len(n.Who))
	for i, uid := range n.Who {
		who[i] = strconv.FormatUint(uid, 10)
	}
	postID := ""
	if n.PostID != 0 {
		postID = strconv.FormatUint(n.PostID, 10)
	}

	return NotificationObject{
		ID:        strconv.FormatUint(n.ID, 10),
		UID:       strconv.FormatUint(n.UID, 10),
		Who:       who,
		WhoCount:  n.WhoCount,
		ActType:   n.ActType,
		PostType:  n.PostType,
		PostID:    postID,
		IsRead:    n.IsRead,
		CreatedAt: n.CreatedAt,
		UpdatedAt: n.UpdatedAt,
	}
}
