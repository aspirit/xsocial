// Package model defines MongoDB data models for the application.
// This file contains the Notification model and the event that feeds it.
package model

import "github.com/aspirit/xsocial/pkg/utils"

// NotificationEventEntityType represents the type of entity the event relates to.
type NotificationEventEntityType int8

const (
	NotificationEventEntityPost NotificationEventEntityType = 1
	NotificationEventEntityUser NotificationEventEntityType = 2
)

// NotificationEvent is one event to apply to a recipient's inbox: "ActorID did
// ActType on (PostType, PostID), notify UID". EventID is the idempotency key;
// 0 means the producer supplied none and every delivery counts.
type NotificationEvent struct {
	UID      uint64
	ActorID  uint64
	ActType  int8
	PostType PostType
	PostID   uint64
	EventID  uint64
}

// Notification is one inbox entry. Events for the same (UID, ActType, PostType,
// PostID) merge into the single unread entry for that target until the user
// reads it; a unique partial index on unread entries enforces the "single".
//
// Who keeps up to N distinct actors for display ("A, B and 3 others"), WhoCount
// counts every merged event, and EventIDs remembers the applied event ids
// (bounded to the most recent ones). A unique index over (uid, event_ids) spans
// every entry, read or unread: a write that would record an id a second time is
// rejected by the database itself, so a redelivered event is a no-op without
// any check-then-act. Entries created without an event id carry no EventIDs
// field at all (omitempty), which keeps them out of that index.
type Notification struct {
	ID        uint64   `bson:"_id" json:"id"`
	UID       uint64   `bson:"uid" json:"uid"`
	Who       []uint64 `bson:"who" json:"who"`
	WhoCount  int64    `bson:"who_count" json:"whoCount"`
	ActType   int8     `bson:"act_type" json:"actType"`
	PostType  PostType `bson:"post_type" json:"postType"`
	PostID    uint64   `bson:"post_id" json:"postId"`
	IsRead    bool     `bson:"is_read" json:"isRead"`
	EventIDs  []uint64 `bson:"event_ids,omitempty" json:"-"`
	CreatedAt int64    `bson:"created_at" json:"createdAt"` // first event
	UpdatedAt int64    `bson:"updated_at" json:"updatedAt"` // latest merged event
}

func (model *Notification) GetMongoCollection() string {
	return utils.InterfaceUtil.SnakeName(model)
}

// NewNotification creates an unread entry holding exactly the event ev.
func NewNotification(id uint64, ev NotificationEvent) *Notification {
	now := utils.TimeUtil.Now()
	n := &Notification{
		ID:        id,
		UID:       ev.UID,
		Who:       []uint64{ev.ActorID},
		WhoCount:  1,
		ActType:   ev.ActType,
		PostType:  ev.PostType,
		PostID:    ev.PostID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if ev.EventID != 0 {
		n.EventIDs = []uint64{ev.EventID}
	}

	return n
}
