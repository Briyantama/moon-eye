package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"moon-eye/backend/pkg/shared/uuidx"
	sqlcdb "moon-eye/backend/internal/db/sqlc"
)

//go:generate mockery --config=../../.mockery_v3.yml

// ChangeEvent represents the minimal data needed to write a change_events row.
type ChangeEvent struct {
	EntityType string
	EntityID   string
	UserID     string
	OpType     string
	Payload    []byte
	Version    int64
}

// ChangeEventRepository abstracts writing change events to the database.
// It is db-layer only and is designed to be used within or outside a
// transaction, depending on the concrete implementation.
type ChangeEventRepository interface {
	Insert(ctx context.Context, tx pgx.Tx, e ChangeEvent) error
}

// SQLCChangeEventRepository writes change events using pgx + sqlc.
type SQLCChangeEventRepository struct {
	pool *pgxpool.Pool
}

// NewChangeEventRepository is the constructor used by the repository container.
func NewChangeEventRepository(pool *pgxpool.Pool, _ *sqlcdb.Queries) ChangeEventRepository {
	return &SQLCChangeEventRepository{pool: pool}
}

// Insert persists a change event, participating in the given transaction when
// tx is non-nil, or using the pool directly otherwise.
func (r *SQLCChangeEventRepository) Insert(ctx context.Context, tx pgx.Tx, e ChangeEvent) error {
	var dbtx sqlcdb.DBTX
	if tx != nil {
		dbtx = tx
	} else {
		dbtx = r.pool
	}

	q := sqlcdb.New(dbtx)

	entityUUID, err := uuidx.StringToPGUUID(e.EntityID)
	if err != nil {
		return fmt.Errorf("invalid entity_id %q: %w", e.EntityID, err)
	}

	userUUID, err := uuidx.StringToPGUUID(e.UserID)
	if err != nil {
		return fmt.Errorf("invalid user_id %q: %w", e.UserID, err)
	}

	return q.InsertChangeEvent(ctx, sqlcdb.InsertChangeEventParams{
		EntityType: e.EntityType,
		EntityID:   entityUUID,
		UserID:     userUUID,
		OpType:     e.OpType,
		Version:    e.Version,
		Payload:    e.Payload,
	})
}


