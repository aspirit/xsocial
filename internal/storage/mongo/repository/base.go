// Package repository provides v2 MongoDB repository implementations
// using the new storage architecture with type-safe field definitions
package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	// Updated to mongo-go-driver v2 import paths.
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options" // Ensure this is correctly imported
	"go.uber.org/zap"                              // Assuming this is still used by your logger package

	internalerr "github.com/aspirit/xsocial/internal/errors"
	"github.com/aspirit/xsocial/pkg/logger" // User's logger package
)

// Base provides common MongoDB operations with context-aware logging.
// It is generic, allowing for type-safe operations on specific document types.
type Base[T any] struct {
	collection *mongo.Collection
}

// NewBase creates a new generic base repository instance.
// It requires a MongoDB database and collection name.
func NewBase[T any](
	database *mongo.Database,
	collectionName string,
) *Base[T] {
	collection := database.Collection(collectionName)

	return &Base[T]{
		collection: collection,
	}
}

// Collection returns the underlying MongoDB collection.
func (r *Base[T]) Collection() *mongo.Collection {
	return r.collection
}

// FindOne finds a single document by filter and decodes it into a T type.
// The result is returned as a pointer to T. If no document is found, it returns nil, internalerr.ErrRecordNotFound.
func (r *Base[T]) FindOne(ctx context.Context, filter bson.M, opts ...options.Lister[options.FindOneOptions]) (*T, error) {
	log := logger.LoggerFromContext(ctx) // Retrieve logger from context
	log.Debug("Base.FindOne", zap.Any("filter", filter))

	var result T
	err := r.collection.FindOne(ctx, filter, opts...).Decode(&result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			log.Debug("Document not found", zap.Any("filter", filter))

			return nil, internalerr.ErrRecordNotFound
		}
		log.Error("FindOne failed", zap.Error(err), zap.Any("filter", filter))

		return nil, fmt.Errorf("failed to decode document: %w", err)
	}

	log.Debug("FindOne successful", zap.Any("filter", filter))

	return &result, nil
}

// FindMany finds multiple documents matching the filter and decodes them into a slice of T.
func (r *Base[T]) FindMany(ctx context.Context, filter bson.M, opts ...options.Lister[options.FindOptions]) ([]T, error) {
	log := logger.LoggerFromContext(ctx) // Retrieve logger from context
	log.Debug("Base.Find", zap.Any("filter", filter))

	cursor, err := r.collection.Find(ctx, filter, opts...)
	if err != nil {
		log.Error("Find failed", zap.Error(err), zap.Any("filter", filter))

		return nil, fmt.Errorf("failed to find documents: %w", err)
	}
	defer func() {
		if err := cursor.Close(ctx); err != nil {
			log.Error("failed to close cursor", zap.Error(err))
		}
	}()

	var results []T
	if err := cursor.All(ctx, &results); err != nil {
		log.Error("Cursor decode failed", zap.Error(err))

		return nil, fmt.Errorf("failed to decode cursor results: %w", err)
	}

	log.Debug("Find successful", zap.Any("filter", filter))

	return results, nil
}

// Find is an alias for FindMany.
// It is provided for backward compatibility.
// It is recommended to use FindMany instead.
// Deprecated: Use FindMany instead.
func (r *Base[T]) Find(ctx context.Context, filter bson.M, opts ...options.Lister[options.FindOptions]) ([]T, error) {
	return r.FindMany(ctx, filter, opts...)
}

// InsertOne inserts a single document of type T.
func (r *Base[T]) InsertOne(ctx context.Context, document T, opts ...options.Lister[options.InsertOneOptions]) (*mongo.InsertOneResult, error) {
	log := logger.LoggerFromContext(ctx) // Retrieve logger from context
	log.Debug("Base.InsertOne")

	result, err := r.collection.InsertOne(ctx, document, opts...)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			log.Warn("Duplicate key error", zap.Error(err))

			return nil, fmt.Errorf("%w: %v", internalerr.ErrDuplicateRecord, err)
		}
		log.Error("InsertOne failed", zap.Error(err))

		return nil, fmt.Errorf("failed to insert document: %w", err)
	}

	log.Debug("InsertOne successful", zap.Any("insertedId", result.InsertedID))

	return result, nil
}

// UpdateOne updates a single document matching the filter.
func (r *Base[T]) UpdateOne(ctx context.Context, filter bson.M, update bson.M, opts ...options.Lister[options.UpdateOneOptions]) (*mongo.UpdateResult, error) {
	log := logger.LoggerFromContext(ctx) // Retrieve logger from context
	log.Debug("Base.UpdateOne", zap.Any("filter", filter))

	result, err := r.collection.UpdateOne(ctx, filter, update, opts...)
	if err != nil {
		log.Error("UpdateOne failed", zap.Error(err), zap.Any("filter", filter))

		return nil, fmt.Errorf("failed to update document: %w", err)
	}

	log.Debug("UpdateOne successful",
		zap.Any("filter", filter),
		zap.Int64("matchedCount", result.MatchedCount),
		zap.Int64("modifiedCount", result.ModifiedCount))

	return result, nil
}

// UpdateMany updates multiple documents matching the filter.
func (r *Base[T]) UpdateMany(ctx context.Context, filter bson.M, update bson.M, opts ...options.Lister[options.UpdateManyOptions]) (*mongo.UpdateResult, error) {
	log := logger.LoggerFromContext(ctx) // Retrieve logger from context
	log.Debug("Base.UpdateMany", zap.Any("filter", filter))

	result, err := r.collection.UpdateMany(ctx, filter, update, opts...)
	if err != nil {
		log.Error("UpdateMany failed", zap.Error(err), zap.Any("filter", filter))

		return nil, fmt.Errorf("failed to update documents: %w", err)
	}

	log.Debug("UpdateMany successful",
		zap.Any("filter", filter),
		zap.Int64("matchedCount", result.MatchedCount),
		zap.Int64("modifiedCount", result.ModifiedCount))

	return result, nil
}

// UpdateByID updates a document by ID.
func (r *Base[T]) UpdateByID(ctx context.Context, id any, update bson.M, opts ...options.Lister[options.UpdateOneOptions]) (*mongo.UpdateResult, error) {
	filter := bson.M{"_id": id}

	return r.UpdateOne(ctx, filter, update, opts...)
}

// DeleteOne deletes a single document matching the filter.
func (r *Base[T]) DeleteOne(ctx context.Context, filter bson.M, opts ...options.Lister[options.DeleteOneOptions]) (*mongo.DeleteResult, error) {
	log := logger.LoggerFromContext(ctx) // Retrieve logger from context
	log.Debug("Base.DeleteOne", zap.Any("filter", filter))

	result, err := r.collection.DeleteOne(ctx, filter, opts...)
	if err != nil {
		log.Error("DeleteOne failed", zap.Error(err), zap.Any("filter", filter))

		return nil, fmt.Errorf("failed to delete document: %w", err)
	}

	log.Debug("DeleteOne successful",
		zap.Any("filter", filter),
		zap.Int64("deletedCount", result.DeletedCount))

	return result, nil
}

// DeleteMany deletes multiple documents matching the filter.
func (r *Base[T]) DeleteMany(ctx context.Context, filter bson.M, opts ...options.Lister[options.DeleteManyOptions]) (*mongo.DeleteResult, error) {
	log := logger.LoggerFromContext(ctx) // Retrieve logger from context
	log.Debug("Base.DeleteMany", zap.Any("filter", filter))

	result, err := r.collection.DeleteMany(ctx, filter, opts...)
	if err != nil {
		log.Error("DeleteMany failed", zap.Error(err), zap.Any("filter", filter))

		return nil, fmt.Errorf("failed to delete documents: %w", err)
	}

	log.Debug("DeleteMany successful",
		zap.Any("filter", filter),
		zap.Int64("deletedCount", result.DeletedCount))

	return result, nil
}

// DeleteByID deletes a document by ID.
func (r *Base[T]) DeleteByID(ctx context.Context, id any, opts ...options.Lister[options.DeleteOneOptions]) (*mongo.DeleteResult, error) {
	filter := bson.M{"_id": id}

	return r.DeleteOne(ctx, filter, opts...)
}

// CountDocuments counts documents matching the filter.
func (r *Base[T]) CountDocuments(ctx context.Context, filter bson.M, opts ...options.Lister[options.CountOptions]) (int64, error) {
	log := logger.LoggerFromContext(ctx) // Retrieve logger from context
	log.Debug("Base.CountDocuments", zap.Any("filter", filter))

	count, err := r.collection.CountDocuments(ctx, filter, opts...)
	if err != nil {
		log.Error("CountDocuments failed", zap.Error(err), zap.Any("filter", filter))

		return 0, fmt.Errorf("failed to count documents: %w", err)
	}

	log.Debug("CountDocuments successful", zap.Any("filter", filter), zap.Int64("count", count))

	return count, nil
}

// Exists checks if a document exists matching the filter.
func (r *Base[T]) Exists(ctx context.Context, filter bson.M, opts ...options.Lister[options.CountOptions]) (bool, error) {
	count, err := r.CountDocuments(ctx, filter, opts...)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// Aggregate performs aggregation pipeline and decodes results into a slice of T.
func (r *Base[T]) Aggregate(ctx context.Context, pipeline []bson.M, opts ...options.Lister[options.AggregateOptions]) ([]T, error) {
	log := logger.LoggerFromContext(ctx) // Retrieve logger from context
	log.Debug("Base.Aggregate", zap.Any("pipeline", pipeline))

	cursor, err := r.collection.Aggregate(ctx, pipeline, opts...)
	if err != nil {
		log.Error("Aggregate failed", zap.Error(err), zap.Any("pipeline", pipeline))

		return nil, fmt.Errorf("failed to aggregate: %w", err)
	}
	defer func() {
		if err := cursor.Close(ctx); err != nil {
			log.Error("failed to close cursor", zap.Error(err))
		}
	}()

	var results []T
	if err := cursor.All(ctx, &results); err != nil {
		log.Error("Aggregate cursor decode failed", zap.Error(err))

		return nil, fmt.Errorf("failed to decode aggregate cursor: %w", err)
	}

	log.Debug("Aggregate successful", zap.Any("pipeline", pipeline))

	return results, nil
}

// WithTransaction runs fn inside a MongoDB multi-document transaction when the
// deployment supports it (replica set / mongos). On a standalone server — which
// cannot start transactions — it transparently falls back to running fn without
// a transaction (best-effort, no atomicity) so single-node setups keep working.
// fn MUST use the context it is given for all DB operations so they join the
// transaction. This is the groundwork for atomic multi-document writes; enable
// real atomicity by running against a replica set.
func (r *Base[T]) WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	log := logger.LoggerFromContext(ctx)
	session, err := r.collection.Database().Client().StartSession()
	if err != nil {
		log.Warn("WithTransaction: cannot start session, running without transaction", zap.Error(err))

		return fn(ctx)
	}
	defer session.EndSession(ctx)

	_, txErr := session.WithTransaction(ctx, func(sessCtx context.Context) (any, error) {
		return nil, fn(sessCtx)
	})
	if txErr == nil {
		return nil
	}
	if isTransactionsUnsupported(txErr) {
		log.Warn("WithTransaction: deployment does not support transactions, falling back to non-transactional execution", zap.Error(txErr))

		return fn(ctx)
	}

	return fmt.Errorf("transaction failed: %w", txErr)
}

// isTransactionsUnsupported reports whether err indicates the MongoDB deployment
// cannot run multi-document transactions (e.g. a standalone server).
func isTransactionsUnsupported(err error) bool {
	if err == nil {
		return false
	}
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		// 20: IllegalOperation (standalone); 263: OperationNotSupportedInTransaction.
		if cmdErr.Code == 20 || cmdErr.Code == 263 {
			return true
		}
	}
	msg := err.Error()

	return strings.Contains(msg, "Transaction numbers are only allowed on a replica set member or mongos") ||
		strings.Contains(msg, "Transactions are not supported")
}
