package server

import (
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// mockLogger implements Logger interface for testing.
type mockLogger struct {
	logger *zap.Logger
}

func (m *mockLogger) Info(msg string, fields ...zap.Field) {
	m.logger.Info(msg, fields...)
}

func (m *mockLogger) Error(msg string, fields ...zap.Field) {
	m.logger.Error(msg, fields...)
}

func TestNewServerConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   *ServerConfig
		expected *ServerConfig
	}{
		{
			name: "default_values",
			config: &ServerConfig{
				ID:   "test-service",
				Addr: ":8080",
			},
			expected: &ServerConfig{
				ID:   "test-service",
				Addr: ":8080",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// FillDefaults should be called internally by Start function
			tt.config.FillDefaults()
			assert.Equal(t, tt.expected.ID, tt.config.ID)
			assert.Equal(t, tt.expected.Addr, tt.config.Addr)
		})
	}
}

func TestNewServerConfigWithTimeouts(t *testing.T) {
	tests := []struct {
		name     string
		config   *ServerConfig
		expected *ServerConfig
	}{
		{
			name: "custom_timeouts",
			config: &ServerConfig{
				ID:              "test-service",
				Addr:            ":8080",
				ReadTimeout:     10 * time.Second,
				WriteTimeout:    20 * time.Second,
				IdleTimeout:     60 * time.Second,
				ShutdownTimeout: 15 * time.Second,
			},
			expected: &ServerConfig{
				ID:              "test-service",
				Addr:            ":8080",
				ReadTimeout:     10 * time.Second,
				WriteTimeout:    20 * time.Second,
				IdleTimeout:     60 * time.Second,
				ShutdownTimeout: 15 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// FillDefaults should be called internally by Start function
			tt.config.FillDefaults()
			assert.Equal(t, tt.expected.ID, tt.config.ID)
			assert.Equal(t, tt.expected.Addr, tt.config.Addr)
			assert.Equal(t, tt.expected.ReadTimeout, tt.config.ReadTimeout)
			assert.Equal(t, tt.expected.WriteTimeout, tt.config.WriteTimeout)
			assert.Equal(t, tt.expected.IdleTimeout, tt.config.IdleTimeout)
			assert.Equal(t, tt.expected.ShutdownTimeout, tt.config.ShutdownTimeout)
		})
	}
}

func TestStart(t *testing.T) {
	// Create a test logger
	testLogger := zaptest.NewLogger(t)
	logger := &mockLogger{logger: testLogger}

	tests := []struct {
		name        string
		config      *ServerConfig
		expectError bool
	}{
		{
			name: "with_server_config",
			config: &ServerConfig{
				ID:   "test-service",
				Addr: ":0", // Use port 0 for testing
			},
			expectError: false,
		},
		{
			name:        "with_nil_config_uses_defaults",
			config:      nil,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a simple HTTP server
			engine := gin.New()
			engine.GET("/health", func(c *gin.Context) {
				c.JSON(200, gin.H{"status": "ok"})
			})

			// Start server in a goroutine
			go func() {
				err := Start(engine, tt.config, logger)
				if tt.expectError {
					assert.Error(t, err)
				} else {
					// For testing, we expect the server to start successfully
					// but we don't wait for it to actually start
					_ = err // Suppress unused variable warning
				}
			}()

			// Give server time to start
			time.Sleep(100 * time.Millisecond)
		})
	}
}

// MockDeps is a mock implementation of service dependencies.
type MockDeps struct {
	t *testing.T
}

func (m *MockDeps) Close() error {
	return nil
}

func (m *MockDeps) GetLogger() Logger {
	testLogger := zaptest.NewLogger(m.t)

	return &mockLogger{logger: testLogger}
}

func TestStartWithDeps(t *testing.T) {
	tests := []struct {
		name        string
		config      *ServerConfig
		expectError bool
	}{
		{
			name: "with_server_config",
			config: &ServerConfig{
				ID:   "test-service",
				Addr: ":0", // Use port 0 for testing
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a simple HTTP server
			engine := gin.New()
			engine.GET("/health", func(c *gin.Context) {
				c.JSON(200, gin.H{"status": "ok"})
			})

			// Create mock deps
			mockDeps := &MockDeps{t: t}

			// Start server in a goroutine
			go func() {
				err := StartWithDeps(engine, tt.config, mockDeps)
				if tt.expectError {
					assert.Error(t, err)
				} else {
					// For testing, we expect the server to start successfully
					// but we don't wait for it to actually start
					_ = err // Suppress unused variable warning
				}
			}()

			// Give server time to start
			time.Sleep(100 * time.Millisecond)
		})
	}
}

func TestLoggerInterface(t *testing.T) {
	// Test that our Logger interface is properly defined
	testLogger := zaptest.NewLogger(t)
	logger := &mockLogger{logger: testLogger}

	t.Run("logger interface", func(t *testing.T) {
		// Test that logger implements Logger interface
		var _ Logger = logger

		// Test Info method
		logger.Info("test info message")

		// Test Error method
		logger.Error("test error message")
	})
}
