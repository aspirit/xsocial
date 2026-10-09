// Package indexes declares every MongoDB index the application relies on and
// creates them idempotently at startup (services call EnsureIndexes once).
//
// Keeping index definitions in code (next to the queries that need them) means a
// new query and its index ship in the same change, and CI's integration tests run
// against exactly the indexes production uses.
//
// Idempotency: CreateMany skips an index only when key pattern and options are
// identical to an existing one; changing a definition requires dropping the old
// index first (manual migration step, intentionally not automated).
package indexes

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	mongoCollections "github.com/aspirit/xsocial/internal/storage/mongo"
	"github.com/aspirit/xsocial/internal/storage/mongo/field"
)

// Index names the repositories need to tell apart duplicate-key errors.
const (
	// UnreadTargetUniq: at most one unread notification per (uid, act_type, post_type, post_id).
	UnreadTargetUniq = "unread_target_uniq"
	// EventUniq: an event id appears on at most one notification per uid, read or unread.
	EventUniq = "uid_event_ids_uniq"
)

// EnsureIndexes creates all indexes on db.
func EnsureIndexes(ctx context.Context, db *mongo.Database) error {
	collections := []struct {
		name    string
		indexes []mongo.IndexModel
	}{
		{mongoCollections.USER_INFO, userInfoIndexes()},
		{mongoCollections.POST, postIndexes()},
		{mongoCollections.POST_LIKE, postLikeIndexes()},
		{mongoCollections.NOTIFICATION, notificationIndexes()},
		// job_lease needs only the default _id index (the job name).
	}

	for _, c := range collections {
		if _, err := db.Collection(c.name).Indexes().CreateMany(ctx, c.indexes); err != nil {
			return fmt.Errorf("ensure indexes for %q: %w", c.name, err)
		}
	}

	return nil
}

// userInfoIndexes: login looks users up by username, which must be unique.
func userInfoIndexes() []mongo.IndexModel {
	return []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: field.UserInfo.Username, Value: 1}},
			Options: options.Index().SetUnique(true).SetName("username_uniq"),
		},
	}
}

// postIndexes: profile pages page through one user's posts by _id DESC.
// The global feed uses the default _id index.
func postIndexes() []mongo.IndexModel {
	return []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: field.Post.UID, Value: 1}, {Key: field.Post.ID, Value: -1}},
			Options: options.Index().SetName("uid_id_idx"),
		},
	}
}

// postLikeIndexes: _id is "<postID>:<uid>" (idempotent upsert); counting by
// post for reconciliation needs post_id.
func postLikeIndexes() []mongo.IndexModel {
	return []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: field.PostLike.PostID, Value: 1}},
			Options: options.Index().SetName("post_id_idx"),
		},
	}
}

// notificationIndexes: inbox listing (uid, updated_at DESC), the unread badge
// (uid, is_read), and the two invariants the consumer relies on:
//
//   - EventUniq (unique, multikey): an event id lives on at most one entry per
//     uid, read or unread. An insert or merge that would record an id a second
//     time fails atomically — that, not a lookup, is what makes redelivery a
//     no-op. Partial on "event_ids exists" so entries without ids (omitempty)
//     do not collide on an absent key.
//   - UnreadTargetUniq (unique, partial): at most ONE unread entry per target,
//     so a concurrent "create or merge" converges: the loser's insert fails and
//     it merges.
func notificationIndexes() []mongo.IndexModel {
	return []mongo.IndexModel{
		{
			Keys: bson.D{{Key: field.Notification.UID, Value: 1}, {Key: field.Notification.EventIDs, Value: 1}},
			Options: options.Index().
				SetUnique(true).
				SetPartialFilterExpression(bson.D{{Key: field.Notification.EventIDs, Value: bson.D{{Key: "$exists", Value: true}}}}).
				SetName(EventUniq),
		},
		{
			Keys:    bson.D{{Key: field.Notification.UID, Value: 1}, {Key: field.Notification.UpdatedAt, Value: -1}},
			Options: options.Index().SetName("uid_updated_idx"),
		},
		{
			Keys:    bson.D{{Key: field.Notification.UID, Value: 1}, {Key: field.Notification.IsRead, Value: 1}},
			Options: options.Index().SetName("uid_read_idx"),
		},
		{
			Keys: bson.D{
				{Key: field.Notification.UID, Value: 1},
				{Key: field.Notification.ActType, Value: 1},
				{Key: field.Notification.PostType, Value: 1},
				{Key: field.Notification.PostID, Value: 1},
			},
			Options: options.Index().
				SetUnique(true).
				SetPartialFilterExpression(bson.D{{Key: field.Notification.IsRead, Value: false}}).
				SetName(UnreadTargetUniq),
		},
	}
}
