// Package repository provides MongoDB repository implementations.
// This file contains the Notification repository (per-user inbox).
package repository

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	internalerr "github.com/aspirit/xsocial/internal/errors"
	"github.com/aspirit/xsocial/internal/model"
	"github.com/aspirit/xsocial/internal/storage/interfaces"
	mongoCollections "github.com/aspirit/xsocial/internal/storage/mongo"
	"github.com/aspirit/xsocial/internal/storage/mongo/field"
	"github.com/aspirit/xsocial/internal/storage/mongo/indexes"
	"github.com/aspirit/xsocial/pkg/utils"
)

// NotificationRepository handles notification persistence.
type NotificationRepository struct {
	*Base[model.Notification]
}

// NewNotificationRepository creates a new notification repository.
func NewNotificationRepository(database *mongo.Database) *NotificationRepository {
	return &NotificationRepository{
		Base: NewBase[model.Notification](database, mongoCollections.NOTIFICATION),
	}
}

// Create inserts n. Two unique indexes can reject it, and the caller needs to
// know which: the unread-target index means "merge instead", the event index
// means "this event was applied before, on an entry the recipient may already
// have read".
func (r *NotificationRepository) Create(ctx context.Context, n *model.Notification) error {
	_, err := r.Collection().InsertOne(ctx, n)
	if err == nil {
		return nil
	}
	switch duplicateKeyIndex(err) {
	case indexes.EventUniq:
		return internalerr.ErrDuplicateEvent
	case "":
		return fmt.Errorf("insert notification: %w", err)
	default:
		return fmt.Errorf("%w: %v", internalerr.ErrDuplicateRecord, err)
	}
}

// MergeEvent implements interfaces.INotificationRepository with a single
// aggregation-pipeline update: concurrent events for one target serialise on the
// document, a redelivery to the same entry leaves it untouched ($in guard), and
// a redelivery whose id already sits on another entry — typically one the
// recipient has read since — is refused by the unique event index. There is no
// read-modify-write and no lookup whose answer could go stale.
func (r *NotificationRepository) MergeEvent(ctx context.Context, ev model.NotificationEvent, whoMax, eventMax int) (interfaces.MergeResult, error) {
	filter := bson.M{
		field.Notification.UID:      ev.UID,
		field.Notification.ActType:  ev.ActType,
		field.Notification.PostType: ev.PostType,
		field.Notification.PostID:   ev.PostID,
		field.Notification.IsRead:   false,
	}

	who := "$" + field.Notification.Who
	events := bson.M{"$ifNull": bson.A{"$" + field.Notification.EventIDs, bson.A{}}}

	// duplicate: this event id is already on this entry (always false without an id).
	var duplicate any = false
	if ev.EventID != 0 {
		duplicate = bson.M{"$in": bson.A{ev.EventID, events}}
	}
	// unlessDuplicate keeps the current value of f for a duplicate, else applies v.
	unlessDuplicate := func(f string, v any) bson.M {
		return bson.M{"$cond": bson.A{duplicate, "$" + f, v}}
	}

	// who gains the actor only while below the cap and not already present.
	addActor := bson.M{"$cond": bson.A{
		bson.M{"$and": bson.A{
			bson.M{"$lt": bson.A{bson.M{"$size": who}, whoMax}},
			bson.M{"$not": bson.A{bson.M{"$in": bson.A{ev.ActorID, who}}}},
		}},
		bson.M{"$concatArrays": bson.A{who, bson.A{ev.ActorID}}},
		who,
	}}

	set := bson.M{
		field.Notification.Who:       unlessDuplicate(field.Notification.Who, addActor),
		field.Notification.WhoCount:  unlessDuplicate(field.Notification.WhoCount, bson.M{"$add": bson.A{bson.M{"$ifNull": bson.A{"$" + field.Notification.WhoCount, 0}}, 1}}),
		field.Notification.UpdatedAt: unlessDuplicate(field.Notification.UpdatedAt, utils.TimeUtil.Now()),
	}
	if ev.EventID != 0 {
		// Append the id and keep only the newest eventMax entries.
		set[field.Notification.EventIDs] = unlessDuplicate(field.Notification.EventIDs,
			bson.M{"$slice": bson.A{bson.M{"$concatArrays": bson.A{events, bson.A{ev.EventID}}}, -eventMax}})
	}

	res, err := r.Collection().UpdateOne(ctx, filter, bson.A{bson.M{"$set": set}})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			// Only EventUniq can be violated here: the id is on another entry.
			return interfaces.MergeAlreadyApplied, nil
		}

		return interfaces.MergeNoUnreadEntry, fmt.Errorf("merge notification event: %w", err)
	}
	switch {
	case res.MatchedCount == 0:
		return interfaces.MergeNoUnreadEntry, nil
	case res.ModifiedCount == 0:
		return interfaces.MergeAlreadyApplied, nil
	default:
		return interfaces.MergeApplied, nil
	}
}

var duplicateKeyIndexRe = regexp.MustCompile(`index: (\S+)`)

// duplicateKeyIndex returns the name of the unique index that rejected a write,
// or "" when err is not a duplicate-key error. The server names the index in
// the write error ("E11000 duplicate key error collection: ... index: <name> ...").
func duplicateKeyIndex(err error) string {
	if !mongo.IsDuplicateKeyError(err) {
		return ""
	}
	var we mongo.WriteException
	if errors.As(err, &we) {
		for _, e := range we.WriteErrors {
			if m := duplicateKeyIndexRe.FindStringSubmatch(e.Message); m != nil {
				return m[1]
			}
		}
	}
	if m := duplicateKeyIndexRe.FindStringSubmatch(err.Error()); m != nil {
		return m[1]
	}

	return ""
}

// FindByUIDWithOffset lists a user's inbox, most recently updated first.
func (r *NotificationRepository) FindByUIDWithOffset(ctx context.Context, uid uint64, limit, offset int64) ([]model.Notification, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: field.Notification.UpdatedAt, Value: -1}}).
		SetLimit(limit).
		SetSkip(offset)

	return r.FindMany(ctx, bson.M{field.Notification.UID: uid}, opts)
}

// MarkAsReadForUser marks notificationID as read only if it belongs to uid and
// reports whether such a notification exists. Ownership is part of the filter,
// so a foreign id is indistinguishable from a missing one.
func (r *NotificationRepository) MarkAsReadForUser(ctx context.Context, uid, notificationID uint64) (bool, error) {
	res, err := r.UpdateOne(ctx,
		bson.M{field.Notification.ID: notificationID, field.Notification.UID: uid},
		bson.M{"$set": bson.M{field.Notification.IsRead: true}},
	)
	if err != nil {
		return false, err
	}

	return res.MatchedCount == 1, nil
}

// MarkAllAsReadByUID marks every unread notification of uid as read.
func (r *NotificationRepository) MarkAllAsReadByUID(ctx context.Context, uid uint64) error {
	_, err := r.UpdateMany(ctx,
		bson.M{field.Notification.UID: uid, field.Notification.IsRead: false},
		bson.M{"$set": bson.M{field.Notification.IsRead: true}},
	)

	return err
}

// CountUnreadByUID counts unread notifications (the badge).
func (r *NotificationRepository) CountUnreadByUID(ctx context.Context, uid uint64) (int64, error) {
	return r.CountDocuments(ctx, bson.M{field.Notification.UID: uid, field.Notification.IsRead: false})
}

// CountByUID counts all notifications of uid.
func (r *NotificationRepository) CountByUID(ctx context.Context, uid uint64) (int64, error) {
	return r.CountDocuments(ctx, bson.M{field.Notification.UID: uid})
}
