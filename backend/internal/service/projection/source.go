package projection

import (
	"context"
	"time"
)

// ChangeEventRow is a minimal view of a change_events row for projection processors.
type ChangeEventRow struct {
	ID         int64
	EntityType string
	EntityID   string
	UserID     string
	OpType     string
	Version    int64
	Payload    []byte
	CreatedAt  time.Time
}

// ChangeEventSource supplies change events for projection processing (e.g. poll by id).
type ChangeEventSource interface {
	// ListSince returns events with id > sinceID, ordered by id asc, at most limit items.
	ListSince(ctx context.Context, sinceID int64, limit int) ([]ChangeEventRow, error)
}
