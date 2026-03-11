package projection

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBCursorStore implements CursorStore using the projection_cursors table.
type DBCursorStore struct {
	pool *pgxpool.Pool
}

// NewDBCursorStore returns a CursorStore backed by the given pool.
func NewDBCursorStore(pool *pgxpool.Pool) *DBCursorStore {
	return &DBCursorStore{pool: pool}
}

func (s *DBCursorStore) Get(ctx context.Context, projectorName string) (int64, error) {
	var lastID int64
	err := s.pool.QueryRow(ctx,
		`SELECT last_event_id FROM projection_cursors WHERE projector_name = $1`,
		projectorName,
	).Scan(&lastID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return lastID, nil
}

func (s *DBCursorStore) Set(ctx context.Context, projectorName string, lastEventID int64) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO projection_cursors (projector_name, last_event_id, updated_at)
		 VALUES ($1, $2, now())
		 ON CONFLICT (projector_name) DO UPDATE SET last_event_id = $2, updated_at = now()`,
		projectorName, lastEventID,
	)
	return err
}
