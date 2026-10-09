// Package redis provides comprehensive tests for the modern Redis client implementation.
// This test suite covers the Redis Manager, Client operations, and enhanced functionality
// including Context Logger support, health checks, and resource management.
package redis

import (
	"context"
	"testing"
	"time"

	"github.com/aspirit/xsocial/internal/config"
	"github.com/aspirit/xsocial/pkg/logger"
)

func TestRedisManager(t *testing.T) {
	// Create a test logger
	log, err := logger.NewLogger(logger.LoggerConfig{
		Level:       "info",
		Development: true,
		ServiceID:   "test-service",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create Redis manager
	manager := NewManager(log)

	// Test configuration
	cfg := config.RedisConfig{
		Address:      "localhost:6379",
		Password:     "",
		Dbname:       0,
		PoolSize:     10,
		MaxRetries:   3,
		MinIdleConns: 5,
		Timeout:      "5s",
	}

	// CreateClient pings the server, so it succeeds only when Redis is reachable.
	// Either way the registry must reflect the outcome exactly.
	_, err = manager.CreateClient("test", cfg)
	want := 1
	if err != nil {
		t.Logf("Redis not available, registry must stay empty: %v", err)
		want = 0
	}

	if clients := manager.ListClients(); len(clients) != want {
		t.Fatalf("expected %d registered client(s), got %v", want, clients)
	}

	health := manager.HealthCheck(context.Background())
	if len(health) != want {
		t.Errorf("expected %d health entries, got %v", want, health)
	}
	if want == 1 && health["test"] != nil {
		t.Errorf("expected a healthy client, got %v", health["test"])
	}

	err = manager.CloseAll()
	if err != nil {
		t.Errorf("CloseAll: %v", err)
	}
	if clients := manager.ListClients(); len(clients) != 0 {
		t.Errorf("expected no clients after CloseAll, got %v", clients)
	}
}

func TestSimpleClient(t *testing.T) {
	// Create a test logger
	log, err := logger.NewLogger(logger.LoggerConfig{
		Level:       "info",
		Development: true,
		ServiceID:   "test-service",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Test configuration
	cfg := config.RedisConfig{
		Address:      "localhost:6379",
		Password:     "",
		Dbname:       0,
		PoolSize:     10,
		MaxRetries:   3,
		MinIdleConns: 5,
		Timeout:      "5s",
	}

	// Test creating a simple client
	_, err = NewSimpleClient(cfg, log)
	if err != nil {
		t.Logf("Expected error when Redis is not available: %v", err)
	}
}

func TestClientOperations(t *testing.T) {
	// This test requires a running Redis instance
	// Skip if Redis is not available
	log, err := logger.NewLogger(logger.LoggerConfig{
		Level:       "info",
		Development: true,
		ServiceID:   "test-service",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	cfg := config.RedisConfig{
		Address:      "localhost:6379",
		Password:     "",
		Dbname:       0,
		PoolSize:     10,
		MaxRetries:   3,
		MinIdleConns: 5,
		Timeout:      "5s",
	}

	manager := NewManager(log)
	client, err := manager.CreateClient("test", cfg)
	if err != nil {
		t.Skip("Skipping test: Redis not available")
	}

	ctx := context.Background()

	// Test SetWithTTL
	err = client.SetWithTTL(ctx, "test:key", "test_value", 10*time.Second)
	if err != nil {
		t.Errorf("Failed to set key with TTL: %v", err)
	}

	// Test GetWithLogging
	value, err := client.GetWithLogging(ctx, "test:key")
	if err != nil {
		t.Errorf("Failed to get key: %v", err)
	}
	if value != "test_value" {
		t.Errorf("Expected 'test_value', got '%s'", value)
	}

	// Test DelWithLogging
	err = client.DelWithLogging(ctx, "test:key")
	if err != nil {
		t.Errorf("Failed to delete key: %v", err)
	}

	// Clean up
	if err := manager.CloseAll(); err != nil {
		t.Errorf("Failed to close manager: %v", err)
	}
}

// TestManagerConcurrency tests concurrent access to the Redis manager.
func TestManagerConcurrency(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggerConfig{
		Level:       "info",
		Development: true,
		ServiceID:   "test-service",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	manager := NewManager(log)
	cfg := config.RedisConfig{
		Address:      "localhost:6379",
		Password:     "",
		Dbname:       0,
		PoolSize:     10,
		MaxRetries:   3,
		MinIdleConns: 5,
		Timeout:      "5s",
	}

	// Test concurrent client creation
	done := make(chan bool, 2)

	go func() {
		defer func() { done <- true }()
		_, err := manager.CreateClient("client1", cfg)
		if err != nil {
			t.Logf("Expected error for client1: %v", err)
		}
	}()

	go func() {
		defer func() { done <- true }()
		_, err := manager.CreateClient("client2", cfg)
		if err != nil {
			t.Logf("Expected error for client2: %v", err)
		}
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	// Clean up
	if err := manager.CloseAll(); err != nil {
		t.Errorf("Failed to close manager: %v", err)
	}
}
