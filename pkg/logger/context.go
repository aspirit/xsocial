package logger

import (
	"context"
)

type loggerKey struct{}

// WithLogger injects a logger into the context.
func WithLogger(ctx context.Context, logger *Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

// LoggerFromContext gets the logger from the context.
func LoggerFromContext(ctx context.Context) *Logger {
	if log, ok := ctx.Value(loggerKey{}).(*Logger); ok {
		return log
	}

	return NewNopLogger()
}
