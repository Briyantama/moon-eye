package projection

import "context"

// CursorStore reads and writes the last processed event id per projector.
type CursorStore interface {
	Get(ctx context.Context, projectorName string) (lastEventID int64, err error)
	Set(ctx context.Context, projectorName string, lastEventID int64) error
}
