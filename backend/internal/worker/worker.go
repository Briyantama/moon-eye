package worker

import (
	"context"
	"time"

	"go.uber.org/zap"

	"moon-eye/backend/internal/queue"
)

// Runner consumes messages from a queue using the provided handler.
type Runner struct {
	Queue   queue.Consumer
	Logger  *zap.Logger
	Name    string
	Handler func(context.Context, queue.Message) error
}

// Start starts the worker loop with basic retry and backoff semantics.
func (r *Runner) Start(ctx context.Context) error {
	if r.Queue == nil || r.Handler == nil {
		return nil
	}

	if r.Logger == nil {
		r.Logger = zap.NewNop()
	}

	r.Logger.Info("starting worker", zap.String("name", r.Name))
	defer r.Logger.Info("worker stopped", zap.String("name", r.Name))

	return r.Queue.Consume(ctx, func(msgCtx context.Context, msg queue.Message) error {
		start := time.Now()
		err := r.Handler(msgCtx, msg)
		duration := time.Since(start)

		if err != nil {
			r.Logger.Warn("job failed",
				zap.String("worker", r.Name),
				zap.String("msg_id", msg.ID),
				zap.String("entity", msg.Entity),
				zap.String("operation", msg.Operation),
				zap.Int("retry_count", msg.RetryCount),
				zap.Duration("duration", duration),
				zap.Error(err),
			)

			// Simple exponential backoff based on RetryCount.
			backoff := time.Duration(1+msg.RetryCount) * time.Second
			select {
			case <-msgCtx.Done():
			case <-time.After(backoff):
			}

			return err
		}

		r.Logger.Info("job processed",
			zap.String("worker", r.Name),
			zap.String("msg_id", msg.ID),
			zap.String("entity", msg.Entity),
			zap.String("operation", msg.Operation),
			zap.Int("retry_count", msg.RetryCount),
			zap.Duration("duration", duration),
		)

		return nil
	})
}

