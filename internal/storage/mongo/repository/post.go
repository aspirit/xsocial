// Package repository provides MongoDB repository implementations.
// This file contains the Post repository.
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
	"github.com/aspirit/xsocial/pkg/utils"
)

// PostRepository handles post persistence.
type PostRepository struct {
	*Base[model.Post]
}

// NewPostRepository creates a new post repository.
func NewPostRepository(database *mongo.Database) *PostRepository {
	return &PostRepository{
		Base: NewBase[model.Post](database, mongoCollections.POST),
	}
}

// FindByID returns a post by ID (internalerr.ErrRecordNotFound when missing).
func (r *PostRepository) FindByID(ctx context.Context, id uint64) (*model.Post, error) {
	return r.FindOne(ctx, bson.M{field.Post.ID: id})
}

// ListLatest returns posts newest-first using keyset pagination on _id
// (Snowflake IDs are time-ordered, so _id doubles as the sort key).
// beforeID == 0 starts from the newest post.
func (r *PostRepository) ListLatest(ctx context.Context, beforeID uint64, limit int64) ([]model.Post, error) {
	return r.FindMany(ctx, keysetFilter(bson.M{}, beforeID), keysetOptions(limit))
}

// ListByUID returns one user's posts newest-first with keyset pagination.
func (r *PostRepository) ListByUID(ctx context.Context, uid, beforeID uint64, limit int64) ([]model.Post, error) {
	return r.FindMany(ctx, keysetFilter(bson.M{field.Post.UID: uid}, beforeID), keysetOptions(limit))
}

// IncLikeCount atomically adds delta to like_count and bumps like_version.
func (r *PostRepository) IncLikeCount(ctx context.Context, id uint64, delta int64) error {
	_, err := r.UpdateByID(ctx, id, bson.M{
		"$inc": bson.M{field.Post.LikeCount: delta, field.Post.LikeVersion: 1},
		"$set": bson.M{field.Post.UpdatedAt: utils.TimeUtil.Now()},
	})
	if err != nil {
		return fmt.Errorf("inc like_count: %w", err)
	}

	return nil
}

// Create inserts a post.
func (r *PostRepository) Create(ctx context.Context, post *model.Post) error {
	_, err := r.InsertOne(ctx, *post)

	return err
}

// CorrectLikeCount overwrites like_count only while like_version is still the
// one the caller read. Any like or unlike in between bumps the version, so a
// stale correction is refused even when the value happens to match again
// (like then unlike: same value, different version).
func (r *PostRepository) CorrectLikeCount(ctx context.Context, id uint64, version, n int64) (bool, error) {
	res, err := r.UpdateOne(ctx,
		bson.M{field.Post.ID: id, field.Post.LikeVersion: version},
		bson.M{"$set": bson.M{field.Post.LikeCount: n}, "$inc": bson.M{field.Post.LikeVersion: 1}},
	)
	if err != nil {
		return false, fmt.Errorf("correct like_count: %w", err)
	}

	return res.MatchedCount == 1, nil
}

// LikeCounters returns the stored like_count and like_version of every post in ids.
func (r *PostRepository) LikeCounters(ctx context.Context, ids []uint64) (map[uint64]model.LikeCounter, error) {
	posts, err := r.FindMany(ctx, bson.M{field.Post.ID: bson.M{"$in": ids}},
		options.Find().SetProjection(bson.M{field.Post.ID: 1, field.Post.LikeCount: 1, field.Post.LikeVersion: 1}))
	if err != nil {
		return nil, err
	}
	out := make(map[uint64]model.LikeCounter, len(posts))
	for _, p := range posts {
		out[p.ID] = model.LikeCounter{Count: p.LikeCount, Version: p.LikeVersion}
	}

	return out, nil
}

// ScanIDs returns post IDs in ascending order after afterID (batch cursor for jobs).
func (r *PostRepository) ScanIDs(ctx context.Context, afterID uint64, limit int64) ([]uint64, error) {
	posts, err := r.FindMany(ctx, bson.M{field.Post.ID: bson.M{"$gt": afterID}},
		options.Find().SetSort(bson.M{field.Post.ID: 1}).SetLimit(limit).SetProjection(bson.M{field.Post.ID: 1}))
	if err != nil {
		return nil, err
	}
	ids := make([]uint64, len(posts))
	for i, p := range posts {
		ids[i] = p.ID
	}

	return ids, nil
}

func keysetFilter(filter bson.M, beforeID uint64) bson.M {
	if beforeID > 0 {
		filter[field.Post.ID] = bson.M{"$lt": beforeID}
	}

	return filter
}

func keysetOptions(limit int64) options.Lister[options.FindOptions] {
	return options.Find().SetSort(bson.D{{Key: field.Post.ID, Value: -1}}).SetLimit(limit)
}
