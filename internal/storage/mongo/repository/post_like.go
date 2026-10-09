// Package repository provides MongoDB repository implementations.
// This file contains the PostLike repository (user ↔ post like relation).
package repository

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/aspirit/xsocial/internal/model"
	mongoCollections "github.com/aspirit/xsocial/internal/storage/mongo"
	"github.com/aspirit/xsocial/internal/storage/mongo/field"
)

// PostLikeRepository handles the like relation.
type PostLikeRepository struct {
	*Base[model.PostLike]
}

// NewPostLikeRepository creates a new like repository.
func NewPostLikeRepository(database *mongo.Database) *PostLikeRepository {
	return &PostLikeRepository{
		Base: NewBase[model.PostLike](database, mongoCollections.POST_LIKE),
	}
}

// Like records a like idempotently. It returns true only when the relation was
// newly created, so callers bump counters / emit events exactly once even under
// retries or double clicks ($setOnInsert upsert on a deterministic _id — no
// check-then-act race).
func (r *PostLikeRepository) Like(ctx context.Context, like *model.PostLike) (bool, error) {
	res, err := r.Collection().UpdateOne(ctx,
		bson.M{field.PostLike.ID: like.ID},
		bson.M{"$setOnInsert": like},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		// Two truly concurrent upserts of one _id can race on the unique _id
		// index; the loser's duplicate-key error just means "already liked".
		if mongo.IsDuplicateKeyError(err) {
			return false, nil
		}

		return false, fmt.Errorf("upsert like: %w", err)
	}

	return res.UpsertedCount == 1, nil
}

// Unlike removes the relation and reports whether it existed.
func (r *PostLikeRepository) Unlike(ctx context.Context, postID, uid uint64) (bool, error) {
	res, err := r.DeleteOne(ctx, bson.M{field.PostLike.ID: model.PostLikeID(postID, uid)})
	if err != nil {
		return false, err
	}

	return res.DeletedCount == 1, nil
}

// LikedBy returns the subset of postIDs liked by uid (for "liked by me" flags).
func (r *PostLikeRepository) LikedBy(ctx context.Context, uid uint64, postIDs []uint64) (map[uint64]bool, error) {
	ids := make([]string, len(postIDs))
	for i, id := range postIDs {
		ids[i] = model.PostLikeID(id, uid)
	}
	likes, err := r.FindMany(ctx, bson.M{field.PostLike.ID: bson.M{"$in": ids}},
		options.Find().SetProjection(bson.M{field.PostLike.PostID: 1}))
	if err != nil {
		return nil, err
	}
	out := make(map[uint64]bool, len(likes))
	for _, l := range likes {
		out[l.PostID] = true
	}

	return out, nil
}

// CountByPosts returns the true like count per post, computed with an
// aggregation pipeline (source of truth for reconciliation).
func (r *PostLikeRepository) CountByPosts(ctx context.Context, postIDs []uint64) (map[uint64]int64, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{field.PostLike.PostID: bson.M{"$in": postIDs}}}},
		{{Key: "$group", Value: bson.M{"_id": "$" + field.PostLike.PostID, "n": bson.M{"$sum": 1}}}},
	}
	cur, err := r.Collection().Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("aggregate like counts: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()

	var rows []struct {
		PostID uint64 `bson:"_id"`
		N      int64  `bson:"n"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("decode like counts: %w", err)
	}
	out := make(map[uint64]int64, len(rows))
	for _, row := range rows {
		out[row.PostID] = row.N
	}

	return out, nil
}
