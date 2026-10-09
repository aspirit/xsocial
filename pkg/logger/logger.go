// Package logger provides structured logging functionality using Uber's zap logger.
package logger

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger wraps zap.Logger for structured logging.
type Logger struct {
	logger *zap.Logger
}

// LoggerConfig holds logger configuration.
type LoggerConfig struct {
	Level           string `mapstructure:"level,omitempty" json:"level,omitempty" yaml:"level,omitempty"`
	OutputPath      string `mapstructure:"output_path,omitempty" json:"outputPath,omitempty" yaml:"output_path,omitempty"`
	ErrorOutputPath string `mapstructure:"error_output_path,omitempty" json:"errorOutputPath,omitempty" yaml:"error_output_path,omitempty"`
	Format          string `mapstructure:"format,omitempty" json:"format,omitempty" yaml:"format,omitempty"`
	Development     bool   `mapstructure:"development,omitempty" json:"development,omitempty" yaml:"development,omitempty"`
	ServiceID       string `mapstructure:"service_id,omitempty" json:"serviceId,omitempty" yaml:"service_id,omitempty"`
}

// NewLogger creates a new logger instance. This is the primary way to get a Logger.
func NewLogger(cfg LoggerConfig) (*Logger, error) {
	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		level = zapcore.InfoLevel // Default to InfoLevel if parsing fails
	}

	if cfg.OutputPath == "" {
		cfg.OutputPath = "stdout"
	}
	if cfg.Format == "" {
		cfg.Format = "json"
	}

	zapCfg := zap.Config{
		Level:       zap.NewAtomicLevelAt(level),
		Development: cfg.Development, // Set development mode based on config
		Sampling: &zap.SamplingConfig{
			Initial:    100,
			Thereafter: 100,
		},
		Encoding:      cfg.Format,
		EncoderConfig: zap.NewProductionEncoderConfig(), // Production EncoderConfig by default

		// Determine output paths
		OutputPaths: []string{cfg.OutputPath},
		ErrorOutputPaths: func() []string {
			if cfg.ErrorOutputPath != "" {
				return []string{cfg.ErrorOutputPath}
			}

			return []string{cfg.OutputPath}
		}(),
	}

	// Customize EncoderConfig for Development mode for better readability
	if cfg.Development {
		zapCfg.EncoderConfig = zap.NewDevelopmentEncoderConfig()
	}

	l, err := zapCfg.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build zap logger: %w", err)
	}

	// Add service ID field if provided
	if cfg.ServiceID != "" {
		l = l.With(zap.String("service_id", cfg.ServiceID))
	}

	return &Logger{
		logger: l,
	}, nil
}

// NewNopLogger creates and returns a new no-operation Logger.
// This logger performs no operations and produces no output.
// It is useful for testing or as a fallback when a real logger is not configured.
func NewNopLogger() *Logger {
	return &Logger{zap.NewNop()}
}

// NewLoggerWithServiceID creates a new logger instance with service ID.
// This is a convenience method for creating service-aware loggers.
func NewLoggerWithServiceID(cfg LoggerConfig, serviceID string) (*Logger, error) {
	cfg.ServiceID = serviceID

	return NewLogger(cfg)
}

// --- Logger Instance Methods (Recommended for use via Dependency Injection) ---

// Debug logs a debug message with fields.
func (l *Logger) Debug(msg string, fields ...zap.Field) {
	l.logger.Debug(msg, fields...)
}

// Info logs an info message with fields.
func (l *Logger) Info(msg string, fields ...zap.Field) {
	l.logger.Info(msg, fields...)
}

// Warn logs a warn message with fields.
func (l *Logger) Warn(msg string, fields ...zap.Field) {
	l.logger.Warn(msg, fields...)
}

// Error logs an error message with fields.
func (l *Logger) Error(msg string, fields ...zap.Field) {
	l.logger.Error(msg, fields...)
}

// DPanic logs a message at DPanic level. In development, it panics. In production, it's just an error.
func (l *Logger) DPanic(msg string, fields ...zap.Field) {
	l.logger.DPanic(msg, fields...)
}

// Panic logs a message at Panic level and then panics.
func (l *Logger) Panic(msg string, fields ...zap.Field) {
	l.logger.Panic(msg, fields...)
}

// Fatal logs a message at Fatal level and then calls os.Exit(1).
func (l *Logger) Fatal(msg string, fields ...zap.Field) {
	l.logger.Fatal(msg, fields...)
}

// With creates a child logger with additional fields (structured).
func (l *Logger) With(fields ...zap.Field) *Logger {
	return &Logger{
		logger: l.logger.With(fields...),
	}
}

// Named adds a new path segment to the logger's name.
func (l *Logger) Named(name string) *Logger {
	return &Logger{
		logger: l.logger.Named(name),
	}
}

// Sync flushes the logger's buffered entries.
func (l *Logger) Sync() error {
	if err := l.logger.Sync(); err != nil {
		return fmt.Errorf("failed to sync logger: %w", err)
	}

	return nil
}
