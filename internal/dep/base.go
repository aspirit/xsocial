// Package dep provides base dependency management for services
package dep

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"

	"github.com/aspirit/xsocial/internal/config"
	internalerr "github.com/aspirit/xsocial/internal/errors"
	cachestorage "github.com/aspirit/xsocial/internal/storage/cache"
	mongostorage "github.com/aspirit/xsocial/internal/storage/mongo"
	"github.com/aspirit/xsocial/pkg/cache"
	"github.com/aspirit/xsocial/pkg/logger"
	"github.com/aspirit/xsocial/pkg/server"
)

// BaseDeps provides common dependencies that all services need.
type BaseDeps struct {
	// Infrastructure
	Logger *logger.Logger
	Config *config.Config

	// Storage
	Databases   map[string]*mongo.Database // keyed by logical name (mongo.DBCore, ...)
	MongoClient *mongo.Client
	Cache       cache.Cache

	// Context for graceful shutdown
	ShutdownCtx context.Context
	Cancel      context.CancelFunc
}

// NewBaseDeps creates base dependencies with common infrastructure.
func NewBaseDeps(serviceID string, cfg *config.Config) (*BaseDeps, error) {
	// Initialize logger with service ID
	loggerInstance, err := logger.NewLoggerWithServiceID(cfg.Logging, serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	// Create shutdown context
	ctx, cancel := context.WithCancel(context.Background())

	base := &BaseDeps{
		Logger:      loggerInstance,
		Config:      cfg,
		ShutdownCtx: ctx,
		Cancel:      cancel,
	}

	// Initialize common storage
	if err := base.initStorage(serviceID); err != nil {
		cancel()

		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	return base, nil
}

// initStorage initializes all storage components.
func (d *BaseDeps) initStorage(serviceID string) error {
	// Initialize MongoDB
	if err := d.initMongoDB(serviceID); err != nil {
		return fmt.Errorf("failed to initialize MongoDB: %w", err)
	}

	// Initialize cache (Redis/Memorystore)
	if err := d.initCache(); err != nil {
		return fmt.Errorf("failed to initialize cache: %w", err)
	}

	return nil
}

// initMongoDB opens the primary MongoDB client and the databases the service
// declared (config.ServiceDatabaseRequirements). Keys stay logical so callers are
// unaffected when storage.mongodb.database_overrides maps them to other physical
// names (e.g. xs_db → xs_test in integration tests).
func (d *BaseDeps) initMongoDB(serviceID string) error {
	d.Databases = make(map[string]*mongo.Database)

	cfg := &d.Config.Storage.MongoDB.Primary
	if cfg.URI == "" {
		return fmt.Errorf("%w: storage.mongodb.primary.uri", internalerr.ErrMongoURIEmpty)
	}
	client, err := mongostorage.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create MongoDB client: %w", err)
	}
	d.MongoClient = client

	databases := config.GetServiceDatabases(serviceID)
	resolved := make([]string, 0, len(databases))
	for _, dbName := range databases {
		physical := d.Config.Storage.MongoDB.ResolveDatabase(dbName)
		d.Databases[dbName] = client.Database(physical)
		resolved = append(resolved, physical)
	}

	d.Logger.Info("Connected to MongoDB",
		zap.Strings("databases", databases),
		zap.Strings("resolved", resolved),
	)

	return nil
}

// initCache initializes cache backend (Redis/Memorystore).
func (d *BaseDeps) initCache() error {
	if d.Config.Storage.Redis.Address == "" {
		d.Logger.Info("Cache not configured (storage.redis.address empty), skipping initialization")

		return nil
	}

	c, err := cachestorage.NewCache(d.Config.Storage.Cache, d.Config.Storage.Redis, d.Logger)
	if err != nil {
		return fmt.Errorf("failed to create cache: %w", err)
	}

	cacheType := cache.ProviderType(strings.ToLower(strings.TrimSpace(d.Config.Storage.Cache.Type)))
	if cacheType == "" {
		cacheType = cache.ProviderRedis
	}

	// Redis and Memorystore share the same protocol/client implementation.
	// Perform a unified connectivity check while preserving type in logs/errors.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Ping(ctx); err != nil {
		_ = c.Close()

		return fmt.Errorf("failed to connect to %s cache: %w", cacheType, err)
	}

	d.Cache = c
	d.Logger.Info("Cache initialized successfully", zap.String("cache_type", string(cacheType)))

	return nil
}

// GetDatabase returns a database by name.
func (d *BaseDeps) GetDatabase(dbName string) (*mongo.Database, error) {
	db, exists := d.Databases[dbName]
	if !exists {
		return nil, fmt.Errorf("%w: %s", internalerr.ErrDatabaseNotInitialized, dbName)
	}

	return db, nil
}

// GetCache returns the cache client (Redis/Memorystore).
func (d *BaseDeps) GetCache() cache.Cache {
	return d.Cache
}

// GetLogger returns the logger instance
// This implements pkg/server.ServiceDeps interface.
func (d *BaseDeps) GetLogger() server.Logger {
	return d.Logger
}

// closeMongoDB disconnects the MongoDB client.
func (d *BaseDeps) closeMongoDB() error {
	if d.MongoClient == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := d.MongoClient.Disconnect(ctx); err != nil {
		d.Logger.Error("Failed to close MongoDB client", zap.Error(err))

		return fmt.Errorf("close mongodb client: %w", err)
	}
	d.Logger.Info("MongoDB client closed")

	return nil
}

// closeCache closes the cache client.
func (d *BaseDeps) closeCache() error {
	if d.Cache != nil {
		if err := d.Cache.Close(); err != nil {
			d.Logger.Error("Failed to close cache", zap.Error(err))

			return fmt.Errorf("failed to close cache: %w", err)
		}
	}

	return nil
}

// syncLogger syncs the logger to flush buffered entries.
func (d *BaseDeps) syncLogger() {
	if d.Logger != nil {
		if err := d.Logger.Sync(); err != nil && !isIgnorableSyncError(err) {
			d.Logger.Warn("Logger sync warning during shutdown", zap.Error(err))
		}
	}
}

// Close closes all resources.
func (d *BaseDeps) Close() error {
	d.Cancel() // Signal shutdown

	var lastErr error

	// Close MongoDB
	if err := d.closeMongoDB(); err != nil {
		lastErr = err
	}

	// Close cache
	if err := d.closeCache(); err != nil {
		lastErr = err
	}

	// Sync logger to flush buffered entries
	d.syncLogger()

	d.Logger.Info("Base dependencies closed")

	return lastErr
}

// isIgnorableSyncError determines if a sync error is ignorable (e.g., ENOTTY on stdout/stderr).
// This approach is cross-platform and avoids string matching.
// See: https://github.com/uber-go/zap/issues/880 https://github.com/uber-go/zap/issues/991
func isIgnorableSyncError(err error) bool {
	if err == nil {
		return false
	}

	var pathErr *os.PathError
	if errors.Is(err, syscall.ENOTTY) || errors.As(err, &pathErr) {
		return true
	}

	return false
}
