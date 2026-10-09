package mongo

import (
	"context"
	"fmt"
	"time"

	internalerr "github.com/aspirit/xsocial/internal/errors"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

func buildClientOptions(config *ClientConfig) (*options.ClientOptions, *readpref.ReadPref, error) {
	if config == nil {
		return nil, nil, internalerr.ErrMongoConfigNil
	}

	// Set defaults and validate
	config.SetDefaults()
	if err := config.Validate(); err != nil {
		return nil, nil, fmt.Errorf("invalid MongoDB config: %w", err)
	}

	// Build client options with all configuration parameters
	clientOptions := options.Client().ApplyURI(config.URI)

	// Set authentication if provided
	if config.Username != "" || config.Password != "" {
		credential := options.Credential{
			Username:   config.Username,
			Password:   config.Password,
			AuthSource: config.AuthSource,
		}
		clientOptions.SetAuth(credential)
	}

	// Set connection pool options
	if config.PoolSize > 0 {
		clientOptions.SetMaxPoolSize(uint64(config.PoolSize))
	}

	// Set max idle time
	if config.MaxIdleTime > 0 {
		clientOptions.SetMaxConnIdleTime(config.MaxIdleTime)
	}

	// Set retry writes
	clientOptions.SetRetryWrites(config.RetryWrites)

	// Set read preference
	readPref, err := config.GetReadPreference()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse read preference: %w", err)
	}
	clientOptions.SetReadPreference(readPref)

	// Set write concern
	writeConcern, err := config.GetWriteConcern()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse write concern: %w", err)
	}
	clientOptions.SetWriteConcern(writeConcern)

	// Set read concern
	readConcern, err := config.GetReadConcern()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse read concern: %w", err)
	}
	clientOptions.SetReadConcern(readConcern)

	return clientOptions, readPref, nil
}

// NewClient creates a new MongoDB client with the given configuration.
func NewClient(config *ClientConfig) (*mongo.Client, error) {
	clientOptions, readPref, err := buildClientOptions(config)
	if err != nil {
		return nil, err
	}

	// Connect to MongoDB - v2 API change: no context parameter
	client, err := mongo.Connect(clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(ctx, readPref); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	return client, nil
}

// NewClientFromURI creates a new MongoDB client with just URI (for backward compatibility).
func NewClientFromURI(uri string) (*mongo.Client, error) {
	config := &ClientConfig{
		URI: uri,
	}

	return NewClient(config)
}
