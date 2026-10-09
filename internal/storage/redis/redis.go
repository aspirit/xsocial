// Package redis provides a modern, production-ready Redis client implementation
// with enhanced features for the backend services.
//
// Key features include:
//   - Manager pattern for handling multiple named Redis clients
//   - Context-aware logging using logger.LoggerFromContext
//   - Built-in health monitoring and connection management
//   - OpenTelemetry integration for tracing and metrics
//   - Graceful shutdown and resource cleanup
//
// This package is designed to replace the legacy internal/db/redis package
// while maintaining backward compatibility during migration.
package redis

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/aspirit/xsocial/internal/config"
	internalerr "github.com/aspirit/xsocial/internal/errors"
	"github.com/aspirit/xsocial/pkg/logger"
)

// Client wraps redis.Client with additional functionality.
type Client struct {
	*redis.Client
	logger *logger.Logger
	config config.RedisConfig
}

// Manager manages Redis clients with proper lifecycle management.
type Manager struct {
	clients map[string]*Client
	mutex   sync.RWMutex
	logger  *logger.Logger
}

// NewManager creates a new Redis manager.
func NewManager(logger *logger.Logger) *Manager {
	return &Manager{
		clients: make(map[string]*Client),
		logger:  logger,
	}
}

// CreateClient creates a new Redis client with the given configuration.
func (m *Manager) CreateClient(name string, cfg config.RedisConfig) (*Client, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Check if client already exists
	if client, exists := m.clients[name]; exists {
		return client, nil
	}

	// Create Redis options
	opts := &redis.Options{
		Addr:         cfg.Address,
		Password:     cfg.Password,
		DB:           cfg.Dbname,
		PoolSize:     cfg.PoolSize,
		MaxRetries:   cfg.MaxRetries,
		MinIdleConns: cfg.MinIdleConns,
	}

	// Parse timeout if configured
	if cfg.Timeout != "" {
		timeout, err := time.ParseDuration(cfg.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout format: %w", err)
		}
		opts.ReadTimeout = timeout
		opts.WriteTimeout = timeout
	}

	// Create Redis client
	rdb := redis.NewClient(opts)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		if closeErr := rdb.Close(); closeErr != nil {
			m.logger.Warn("Failed to close Redis client after connection failure", zap.Error(closeErr))
		}

		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	// Enable OpenTelemetry instrumentation
	if err := redisotel.InstrumentTracing(rdb); err != nil {
		m.logger.Warn("Failed to enable Redis tracing", zap.Error(err))
	}

	if err := redisotel.InstrumentMetrics(rdb); err != nil {
		m.logger.Warn("Failed to enable Redis metrics", zap.Error(err))
	}

	// Create wrapped client
	client := &Client{
		Client: rdb,
		logger: m.logger.Named("redis").With(zap.String("client", name)),
		config: cfg,
	}

	// Store client
	m.clients[name] = client

	m.logger.Info("Redis client created successfully",
		zap.String("name", name),
		zap.String("addr", cfg.Address),
		zap.Int("db", cfg.Dbname),
	)

	return client, nil
}

// GetClient returns an existing Redis client.
func (m *Manager) GetClient(name string) (*Client, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	client, exists := m.clients[name]
	if !exists {
		return nil, fmt.Errorf("%w: %s", internalerr.ErrRedisClientNotFound, name)
	}

	return client, nil
}

// CloseClient closes a specific Redis client.
func (m *Manager) CloseClient(name string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	client, exists := m.clients[name]
	if !exists {
		return fmt.Errorf("%w: %s", internalerr.ErrRedisClientNotFound, name)
	}

	if err := client.Close(); err != nil {
		return fmt.Errorf("failed to close Redis client '%s': %w", name, err)
	}

	delete(m.clients, name)

	m.logger.Info("Redis client closed", zap.String("name", name))

	return nil
}

// CloseAll closes all Redis clients.
func (m *Manager) CloseAll() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	var errors []error

	for name, client := range m.clients {
		if err := client.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close client '%s': %w", name, err))
		}
	}

	m.clients = make(map[string]*Client)

	if len(errors) > 0 {
		return fmt.Errorf("%w: %v", internalerr.ErrClosingRedisClients, errors)
	}

	m.logger.Info("All Redis clients closed")

	return nil
}

// ListClients returns the names of all active clients.
func (m *Manager) ListClients() []string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	names := make([]string, 0, len(m.clients))
	for name := range m.clients {
		names = append(names, name)
	}

	return names
}

// HealthCheck checks the health of all Redis clients.
func (m *Manager) HealthCheck(ctx context.Context) map[string]error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	results := make(map[string]error)

	for name, client := range m.clients {
		err := client.Ping(ctx).Err()
		results[name] = err
	}

	return results
}

// Enhanced methods for the Client

// SetWithTTL sets a key with TTL and logs the operation.
func (c *Client) SetWithTTL(ctx context.Context, key string, value any, ttl time.Duration) error {
	// Use context logger if available, fallback to client logger
	log := logger.LoggerFromContext(ctx)
	if log == nil {
		log = c.logger
	}

	err := c.Set(ctx, key, value, ttl).Err()
	if err != nil {
		log.Error("Failed to set Redis key",
			zap.String("key", key),
			zap.Duration("ttl", ttl),
			zap.Error(err),
		)

		return fmt.Errorf("failed to set Redis key: %w", err)
	}

	log.Debug("Redis key set successfully",
		zap.String("key", key),
		zap.Duration("ttl", ttl),
	)

	return nil
}

// GetWithLogging gets a key and logs the operation.
func (c *Client) GetWithLogging(ctx context.Context, key string) (string, error) {
	// Use context logger if available, fallback to client logger
	log := logger.LoggerFromContext(ctx)
	if log == nil {
		log = c.logger
	}

	result, err := c.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			log.Debug("Redis key not found", zap.String("key", key))
		} else {
			log.Error("Failed to get Redis key",
				zap.String("key", key),
				zap.Error(err),
			)
		}

		return "", fmt.Errorf("failed to get Redis key: %w", err)
	}

	log.Debug("Redis key retrieved successfully", zap.String("key", key))

	return result, nil
}

// DelWithLogging deletes a key and logs the operation.
func (c *Client) DelWithLogging(ctx context.Context, keys ...string) error {
	// Use context logger if available, fallback to client logger
	log := logger.LoggerFromContext(ctx)
	if log == nil {
		log = c.logger
	}

	result, err := c.Del(ctx, keys...).Result()
	if err != nil {
		log.Error("Failed to delete Redis keys",
			zap.Strings("keys", keys),
			zap.Error(err),
		)

		return fmt.Errorf("failed to delete Redis keys: %w", err)
	}

	log.Debug("Redis keys deleted successfully",
		zap.Strings("keys", keys),
		zap.Int64("deleted_count", result),
	)

	return nil
}

// NewSimpleClient creates a simple Redis client without manager (for backward compatibility).
func NewSimpleClient(cfg config.RedisConfig, logger *logger.Logger) (*Client, error) {
	manager := NewManager(logger)

	return manager.CreateClient("default", cfg)
}
