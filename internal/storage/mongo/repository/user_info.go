// Package repository provides MongoDB repository implementations.
// This file contains the UserInfo repository.
package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/aspirit/xsocial/internal/model"
	mongoCollections "github.com/aspirit/xsocial/internal/storage/mongo"
	"github.com/aspirit/xsocial/internal/storage/mongo/field"
)

// UserInfoRepository handles account persistence.
type UserInfoRepository struct {
	*Base[model.UserInfo]
}

// NewUserInfoRepository creates a new user repository.
func NewUserInfoRepository(database *mongo.Database) *UserInfoRepository {
	return &UserInfoRepository{
		Base: NewBase[model.UserInfo](database, mongoCollections.USER_INFO),
	}
}

// FindByID returns a user by ID.
func (r *UserInfoRepository) FindByID(ctx context.Context, id uint64) (*model.UserInfo, error) {
	return r.FindOne(ctx, bson.M{field.UserInfo.ID: id})
}

// FindByUsername returns a user by username (unique index, see indexes.EnsureIndexes).
func (r *UserInfoRepository) FindByUsername(ctx context.Context, username string) (*model.UserInfo, error) {
	return r.FindOne(ctx, bson.M{field.UserInfo.Username: username})
}

// Create inserts a user; the unique username index reports a taken name as
// internalerr.ErrDuplicateRecord (translated by Base.InsertOne).
func (r *UserInfoRepository) Create(ctx context.Context, user *model.UserInfo) error {
	_, err := r.InsertOne(ctx, *user)

	return err
}
