package projection

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// Runner runs projection processors in a loop, reading from ChangeEventSource and advancing cursor.
type Runner struct {
	Name      string
	Source    ChangeEventSource
	Cursor    CursorStore
	Processors []Processor
	Logger    *zap.Logger
	BatchSize int
	PollInterval time.Duration
}

// DefaultBatchSize and DefaultPollInterval are used when zero.
const (
	DefaultBatchSize    = 50
	DefaultPollInterval = 2 * time.Second
)

// Start runs the projection loop until ctx is cancelled. It processes events in order,
// runs all processors for each event, then advances the cursor only after a full batch succeeds.
func (r *Runner) Start(ctx context.Context) error {
	if r.Source == nil || r.Cursor == nil || len(r.Processors) == 0 {
		return nil
	}
	batchSize := r.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	pollInterval := r.PollInterval
	if pollInterval <= 0 {
		pollInterval = DefaultPollInterval
	}
	logger := r.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	logger.Info("projection runner started", zap.String("name", r.Name))
	defer logger.Info("projection runner stopped", zap.String("name", r.Name))

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		lastID, err := r.Cursor.Get(ctx, r.Name)
		if err != nil {
			logger.Warn("get cursor failed", zap.String("name", r.Name), zap.Error(err))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(pollInterval):
			}
			continue
		}

		events, err := r.Source.ListSince(ctx, lastID, batchSize)
		if err != nil {
			logger.Warn("list events failed", zap.String("name", r.Name), zap.Error(err))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(pollInterval):
			}
			continue
		}

		if len(events) == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(pollInterval):
			}
			continue
		}

		var maxID int64
		ok := true
		for _, ev := range events {
			for _, proc := range r.Processors {
				if err := proc.Process(ctx, ev); err != nil {
					logger.Warn("processor failed", zap.String("name", r.Name), zap.Int64("event_id", ev.ID), zap.Error(err))
					ok = false
					break
				}
			}
			if !ok {
				break
			}
			if ev.ID > maxID {
				maxID = ev.ID
			}
		}

		if ok && maxID > 0 {
			if err := r.Cursor.Set(ctx, r.Name, maxID); err != nil {
				logger.Warn("set cursor failed", zap.String("name", r.Name), zap.Int64("last_id", maxID), zap.Error(err))
			}
		} else if !ok {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(pollInterval):
			}
		}

		// If we got a full batch, continue immediately; otherwise wait before next poll.
		if len(events) < batchSize {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(pollInterval):
			}
		}
	}
}
