package projection

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DBChangeEventSource implements ChangeEventSource by reading from change_events.
type DBChangeEventSource struct {
	pool *pgxpool.Pool
}

// NewDBChangeEventSource returns a ChangeEventSource backed by the given pool.
func NewDBChangeEventSource(pool *pgxpool.Pool) *DBChangeEventSource {
	return &DBChangeEventSource{pool: pool}
}

// ListSince returns events with id > sinceID, ordered by id asc.
const listChangeEventsSinceSQL = `
SELECT id, entity_type, entity_id::text, user_id::text, op_type, version, payload, created_at
FROM change_events
WHERE id > $1
ORDER BY id ASC
LIMIT $2
`

func (s *DBChangeEventSource) ListSince(ctx context.Context, sinceID int64, limit int) ([]ChangeEventRow, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, listChangeEventsSinceSQL, sinceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ChangeEventRow
	for rows.Next() {
		var row ChangeEventRow
		if err := rows.Scan(
			&row.ID,
			&row.EntityType,
			&row.EntityID,
			&row.UserID,
			&row.OpType,
			&row.Version,
			&row.Payload,
			&row.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
