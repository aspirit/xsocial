// Package interfaces defines the data-access contracts that domains depend on.
//
// Domains (internal/domains/*) import these interfaces, never the concrete MongoDB
// repositories, so business logic is unit-testable with in-memory fakes and the
// storage engine can change without touching domain code: nothing here mentions
// MongoDB. Concrete implementations live in internal/storage/mongo/repository and
// are bound to these interfaces by Google Wire (services/*/wire).
package interfaces

import (
	"context"
	"time"

	"github.com/aspirit/xsocial/internal/model"
)

// Transactor runs fn inside a MongoDB transaction when the deployment supports
// it (replica set) and falls back to plain execution otherwise.
type Transactor interface {
	WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// IUserInfoRepository is the account store.
type IUserInfoRepository interface {
	FindByID(ctx context.Context, id uint64) (*model.UserInfo, error)
	FindByUsername(ctx context.Context, username string) (*model.UserInfo, error)
	// Create inserts the user; internalerr.ErrDuplicateRecord when the username is taken.
	Create(ctx context.Context, user *model.UserInfo) error
}

// IPostRepository is the post store.
type IPostRepository interface {
	Transactor
	FindByID(ctx context.Context, id uint64) (*model.Post, error)
	ListLatest(ctx context.Context, beforeID uint64, limit int64) ([]model.Post, error)
	ListByUID(ctx context.Context, uid, beforeID uint64, limit int64) ([]model.Post, error)
	Create(ctx context.Context, post *model.Post) error
	// IncLikeCount adds delta to like_count and bumps like_version.
	IncLikeCount(ctx context.Context, id uint64, delta int64) error
	// CorrectLikeCount sets like_count to n only while like_version still equals
	// version (i.e. nothing changed the counter since that version was read),
	// bumps the version, and reports whether it did.
	CorrectLikeCount(ctx context.Context, id uint64, version, n int64) (bool, error)
	// LikeCounters returns the stored counter and version of every post in ids.
	LikeCounters(ctx context.Context, ids []uint64) (map[uint64]model.LikeCounter, error)
	ScanIDs(ctx context.Context, afterID uint64, limit int64) ([]uint64, error)
}

// IPostLikeRepository is the like-relation store.
type IPostLikeRepository interface {
	Like(ctx context.Context, like *model.PostLike) (bool, error)
	Unlike(ctx context.Context, postID, uid uint64) (bool, error)
	LikedBy(ctx context.Context, uid uint64, postIDs []uint64) (map[uint64]bool, error)
	CountByPosts(ctx context.Context, postIDs []uint64) (map[uint64]int64, error)
}

// MergeResult is the outcome of INotificationRepository.MergeEvent.
type MergeResult int

const (
	// MergeNoUnreadEntry: the target has no unread entry; the caller creates one.
	MergeNoUnreadEntry MergeResult = iota
	// MergeApplied: the event changed the unread entry.
	MergeApplied
	// MergeAlreadyApplied: the event id is already recorded, on this entry or on
	// another (read) one; nothing changed.
	MergeAlreadyApplied
)

// INotificationRepository is the inbox store. Two unique indexes carry its
// guarantees: one unread entry per (uid, act_type, post_type, post_id), and one
// entry per (uid, event id) across read and unread entries alike.
type INotificationRepository interface {
	// Create inserts an unread entry. internalerr.ErrDuplicateRecord: the unread
	// entry for the target already exists (merge instead). internalerr.
	// ErrDuplicateEvent: n's event id is already recorded on some entry.
	Create(ctx context.Context, n *model.Notification) error
	// MergeEvent applies ev to the unread entry for its target in ONE atomic
	// update: the actor joins who (distinct, capped at whoMax), who_count grows by
	// one and the event id is recorded (newest eventMax kept). The update is
	// refused, atomically, when the id is already recorded anywhere.
	MergeEvent(ctx context.Context, ev model.NotificationEvent, whoMax, eventMax int) (MergeResult, error)
	FindByUIDWithOffset(ctx context.Context, uid uint64, limit, offset int64) ([]model.Notification, error)
	MarkAsReadForUser(ctx context.Context, uid, notificationID uint64) (bool, error)
	MarkAllAsReadByUID(ctx context.Context, uid uint64) error
	CountUnreadByUID(ctx context.Context, uid uint64) (int64, error)
	CountByUID(ctx context.Context, uid uint64) (int64, error)
}

// IJobLeaseRepository hands out the leases that keep a scheduled job on one
// worker replica per interval.
type IJobLeaseRepository interface {
	// TryAcquire takes or renews the lease name for holder, valid for ttl. It
	// returns false when another holder's lease is still live.
	TryAcquire(ctx context.Context, name, holder string, ttl time.Duration) (bool, error)
}
