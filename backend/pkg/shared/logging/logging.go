package logging

import (
	"context"

	"go.uber.org/zap"
)

type loggerKey struct{}

// NewLogger creates a new Zap logger configured for production by default.
func NewLogger() (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	// TODO: make log level configurable via shared config.
	return cfg.Build()
}

// WithLogger returns a new context with the given logger attached.
func WithLogger(ctx context.Context, logger *zap.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

// FromContext extracts a logger from context, or returns a no-op logger.
func FromContext(ctx context.Context) *zap.Logger {
	if ctx == nil {
		return zap.NewNop()
	}
	if l, ok := ctx.Value(loggerKey{}).(*zap.Logger); ok && l != nil {
		return l
	}
	return zap.NewNop()
}

