package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	syncdomain "moon-eye/backend/internal/sync"
)

// PGShedsConnectionRepository implements SheetsConnectionRepository using pgxpool.
type PGShedsConnectionRepository struct {
	pool *pgxpool.Pool
}

func NewPGShedsConnectionRepository(pool *pgxpool.Pool) *PGShedsConnectionRepository {
	return &PGShedsConnectionRepository{pool: pool}
}

func (r *PGShedsConnectionRepository) GetByID(ctx context.Context, id string) (*syncdomain.SheetsConnection, error) {
	const q = `
		SELECT id, user_id, sheet_id, COALESCE(sheet_range, ''), sync_mode, EXTRACT(EPOCH FROM last_synced_at)
		FROM sheets_connections
		WHERE id = $1
	`
	var (
		conn       syncdomain.SheetsConnection
		lastSynced *float64
	)

	if err := r.pool.QueryRow(ctx, q, id).Scan(
		&conn.ID,
		&conn.UserID,
		&conn.SheetID,
		&conn.SheetRange,
		&conn.SyncMode,
		&lastSynced,
	); err != nil {
		return nil, err
	}

	if lastSynced != nil {
		ts := int64(*lastSynced)
		conn.LastSyncedAt = &ts
	}

	return &conn, nil
}

func (r *PGShedsConnectionRepository) ListActiveByUser(ctx context.Context, userID string) ([]syncdomain.SheetsConnection, error) {
	const q = `
		SELECT id, user_id, sheet_id, COALESCE(sheet_range, ''), sync_mode, EXTRACT(EPOCH FROM last_synced_at)
		FROM sheets_connections
		WHERE user_id = $1
	`

	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []syncdomain.SheetsConnection
	for rows.Next() {
		var (
			conn       syncdomain.SheetsConnection
			lastSynced *float64
		)

		if err := rows.Scan(
			&conn.ID,
			&conn.UserID,
			&conn.SheetID,
			&conn.SheetRange,
			&conn.SyncMode,
			&lastSynced,
		); err != nil {
			return nil, err
		}

		if lastSynced != nil {
			ts := int64(*lastSynced)
			conn.LastSyncedAt = &ts
		}

		result = append(result, conn)
	}

	return result, rows.Err()
}

// PGSheetMappingRepository implements SheetMappingRepository using pgxpool.
type PGSheetMappingRepository struct {
	pool *pgxpool.Pool
}

func NewPGSheetMappingRepository(pool *pgxpool.Pool) *PGSheetMappingRepository {
	return &PGSheetMappingRepository{pool: pool}
}

func (r *PGSheetMappingRepository) ListByConnection(ctx context.Context, connectionID string) ([]syncdomain.SheetMapping, error) {
	const q = `
		SELECT id, connection_id, sheet_column, db_field, transform
		FROM sheet_mappings
		WHERE connection_id = $1
		ORDER BY sheet_column
	`

	rows, err := r.pool.Query(ctx, q, connectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []syncdomain.SheetMapping
	for rows.Next() {
		var m syncdomain.SheetMapping
		if err := rows.Scan(
			&m.ID,
			&m.ConnectionID,
			&m.SheetColumn,
			&m.DBField,
			&m.Transform,
		); err != nil {
			return nil, err
		}
		result = append(result, m)
	}

	return result, rows.Err()
}

// PGChangeEventReader implements ChangeEventReader for the change_events table.
type PGChangeEventReader struct {
	pool *pgxpool.Pool
}

func NewPGChangeEventReader(pool *pgxpool.Pool) *PGChangeEventReader {
	return &PGChangeEventReader{pool: pool}
}

func (r *PGChangeEventReader) ListTransactionEventsSince(ctx context.Context, userID string, sinceVersion int64, limit int) ([]syncdomain.ChangeEvent, error) {
	const q = `
		SELECT id, entity_type, entity_id, user_id, op_type, version, payload, created_at
		FROM change_events
		WHERE user_id = $1
		  AND entity_type = 'transaction'
		  AND version > $2
		ORDER BY version ASC
		LIMIT $3
	`

	rows, err := r.pool.Query(ctx, q, userID, sinceVersion, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []syncdomain.ChangeEvent
	for rows.Next() {
		var ev syncdomain.ChangeEvent
		if err := rows.Scan(
			&ev.ID,
			&ev.EntityType,
			&ev.EntityID,
			&ev.UserID,
			&ev.OpType,
			&ev.Version,
			&ev.Payload,
			&ev.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, ev)
	}

	return result, rows.Err()
}

