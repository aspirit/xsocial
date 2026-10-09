// Package repository provides MongoDB repository implementations.
// This file contains the JobLease repository (one scheduled-job runner per interval).
package repository

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/aspirit/xsocial/internal/model"
	mongoCollections "github.com/aspirit/xsocial/internal/storage/mongo"
	"github.com/aspirit/xsocial/internal/storage/mongo/field"
	"github.com/aspirit/xsocial/pkg/utils"
)

// JobLeaseRepository implements interfaces.IJobLeaseRepository on MongoDB: the
// unique _id (job name) plus one conditional upsert is the whole lock.
type JobLeaseRepository struct {
	*Base[model.JobLease]
}

// NewJobLeaseRepository creates a new job lease repository.
func NewJobLeaseRepository(database *mongo.Database) *JobLeaseRepository {
	return &JobLeaseRepository{
		Base: NewBase[model.JobLease](database, mongoCollections.JOB_LEASE),
	}
}

// TryAcquire takes the lease when it is free (missing or expired) or already
// held by holder (renewal). A live lease owned by someone else makes the upsert
// collide on _id, which is reported as "not acquired", not as an error.
func (r *JobLeaseRepository) TryAcquire(ctx context.Context, name, holder string, ttl time.Duration) (bool, error) {
	now := utils.TimeUtil.Now()
	filter := bson.M{
		field.JobLease.ID: name,
		"$or": bson.A{
			bson.M{field.JobLease.ExpiresAt: bson.M{"$lte": now}},
			bson.M{field.JobLease.Holder: holder},
		},
	}
	update := bson.M{"$set": bson.M{
		field.JobLease.Holder:    holder,
		field.JobLease.ExpiresAt: now + ttl.Milliseconds(),
	}}
	res, err := r.Collection().UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(true))
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return false, nil
		}

		return false, fmt.Errorf("acquire lease %q: %w", name, err)
	}

	return res.MatchedCount == 1 || res.UpsertedCount == 1, nil
}
