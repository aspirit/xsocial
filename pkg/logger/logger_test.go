package logger

import (
	"testing"
)

func TestNewLoggerWithServiceID(t *testing.T) {
	cfg := LoggerConfig{
		Level:       "info",
		OutputPath:  "stdout",
		Format:      "json",
		Development: false,
	}

	serviceID := "xs-api"
	logger, err := NewLoggerWithServiceID(cfg, serviceID)
	if err != nil {
		t.Fatalf("NewLoggerWithServiceID() error = %v", err)
	}

	if logger == nil {
		t.Fatal("NewLoggerWithServiceID() returned nil logger")
	}

	// Test that the logger was created successfully
	logger.Info("Test message with service ID")
}

func TestNewLoggerWithEmptyServiceID(t *testing.T) {
	cfg := LoggerConfig{
		Level:       "info",
		OutputPath:  "stdout",
		Format:      "json",
		Development: false,
	}

	// Empty service ID should still work
	logger, err := NewLoggerWithServiceID(cfg, "")
	if err != nil {
		t.Fatalf("NewLoggerWithServiceID() with empty service ID error = %v", err)
	}

	if logger == nil {
		t.Fatal("NewLoggerWithServiceID() returned nil logger")
	}

	// Test that the logger was created successfully
	logger.Info("Test message without service ID")
}

func TestLoggerConfig_ServiceID(t *testing.T) {
	cfg := LoggerConfig{
		Level:       "info",
		OutputPath:  "stdout",
		Format:      "json",
		Development: false,
		ServiceID:   "xs-test",
	}

	logger, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}

	if logger == nil {
		t.Fatal("NewLogger() returned nil logger")
	}

	// Test that the logger was created successfully with service ID in config
	logger.Info("Test message with service ID from config")
}
