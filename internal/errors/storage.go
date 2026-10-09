// Package errors provides storage layer error definitions.
package errors

import "errors"

// Storage layer errors.
var (
	// Generic storage errors.
	ErrRecordNotFound  = errors.New("record not found")
	ErrDuplicateRecord = errors.New("duplicate record")
	// ErrDuplicateEvent: the write would record an event id that another
	// document already holds (unique index); the event was applied before.
	ErrDuplicateEvent = errors.New("event already applied")

	// MongoDB client configuration.
	ErrMongoConfigNil                 = errors.New("MongoDB config cannot be nil")
	ErrMongoURIEmpty                  = errors.New("MongoDB URI cannot be empty")
	ErrPoolSizeNegative               = errors.New("pool size cannot be negative")
	ErrMinPoolSizeNegative            = errors.New("min pool size cannot be negative")
	ErrMinPoolSizeGreaterThanMax      = errors.New("min pool size cannot be greater than max pool size")
	ErrMaxIdleTimeNegative            = errors.New("max idle time cannot be negative")
	ErrConnectTimeoutNegative         = errors.New("connect timeout cannot be negative")
	ErrSocketTimeoutNegative          = errors.New("socket timeout cannot be negative")
	ErrServerSelectionTimeoutNegative = errors.New("server selection timeout cannot be negative")
	ErrInvalidReadPreference          = errors.New("invalid read preference")
	ErrInvalidWriteConcern            = errors.New("invalid write concern")
	ErrInvalidReadConcern             = errors.New("invalid read concern")
	ErrInvalidCompressor              = errors.New("invalid compressor")

	// Redis clients.
	ErrRedisClientNotFound = errors.New("redis client not found")
	ErrClosingRedisClients = errors.New("errors closing Redis clients")

	// Database instances.
	ErrDatabaseNotInitialized = errors.New("database not initialized")
)
