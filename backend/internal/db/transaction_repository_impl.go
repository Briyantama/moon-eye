package db

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gofrs/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	sqlcdb "moon-eye/backend/internal/db/sqlc"
	"moon-eye/backend/pkg/shared/pgtypex"
	"moon-eye/backend/pkg/shared/uuidx"
)

// PGXTransactionRepository is a pgx/pgxpool-backed implementation of
// TransactionRepository. It satisfies the requirement that all multi-write
// operations can participate in a single PostgreSQL transaction by accepting
// an optional pgx.Tx in each method.
type PGXTransactionRepository struct {
	pool *pgxpool.Pool
	queries *sqlcdb.Queries
}

// NewPGXTransactionRepository constructs a new TransactionRepository backed by
// the given pgx connection pool (uses sqlc Queries built from the same pool).
func NewPGXTransactionRepository(pool *pgxpool.Pool) *PGXTransactionRepository {
	return &PGXTransactionRepository{
		pool: pool,
	}
}

// NewTransactionRepository is the public constructor used by the repository
// container. Uses the provided sqlc Queries when non-nil; otherwise builds from pool.
func NewTransactionRepository(pool *pgxpool.Pool, q *sqlcdb.Queries) TransactionRepository {
	if q != nil {
		return &PGXTransactionRepository{pool: pool, queries: q}
	}
	return NewPGXTransactionRepository(pool)
}

func (r *PGXTransactionRepository) q(tx pgx.Tx) *sqlcdb.Queries {
	if tx != nil {
		return sqlcdb.New(tx)
	}
	return sqlcdb.New(r.pool)
}

// Create inserts a new transaction row and returns the created record.
func (r *PGXTransactionRepository) Create(ctx context.Context, tx pgx.Tx, params CreateTransactionParams) (*Transaction, error) {
	metaBytes, err := json.Marshal(params.Metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}

	amount, err := pgtypex.NumericFromFloat64(params.Amount)
	if err != nil {
		return nil, fmt.Errorf("convert amount: %w", err)
	}
	occurredAt, err := pgtypex.TimestamptzFromTime(params.OccurredAt)
	if err != nil {
		return nil, fmt.Errorf("convert occurred_at: %w", err)
	}
	lastModified, err := pgtypex.TimestamptzFromTime(params.LastModified)
	if err != nil {
		return nil, fmt.Errorf("convert last_modified: %w", err)
	}
	description, err := pgtypex.TextFromStringPtr(params.Description)
	if err != nil {
		return nil, fmt.Errorf("convert description: %w", err)
	}
	sheetsRowID, err := pgtypex.TextFromStringPtr(params.SheetsRowID)
	if err != nil {
		return nil, fmt.Errorf("convert sheets_row_id: %w", err)
	}

	arg := sqlcdb.CreateTransactionParams{
		UserID:       uuidx.UUIDToPG(params.UserID),
		AccountID:    uuidx.UUIDToPG(params.AccountID),
		Amount:       amount,
		Currency:     params.Currency,
		Type:         params.Type,
		CategoryID:   uuidx.UUIDPtrToPG(params.CategoryID),
		Description:  description,
		OccurredAt:   occurredAt,
		Metadata:     metaBytes,
		Version:      params.Version,
		LastModified: lastModified,
		Source:       params.Source,
		SheetsRowID:  sheetsRowID,
		Deleted:      params.Deleted,
	}
	row, err := r.q(tx).CreateTransaction(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("create transaction: %w", err)
	}
	return sqlcTransactionToDB(&row)
}

// GetByID returns a single transaction by its ID (no user filter).
func (r *PGXTransactionRepository) GetByID(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Transaction, error) {
	row, err := r.q(tx).GetTransactionByIDOnly(ctx, uuidx.UUIDToPG(id))
	if err != nil {
		return nil, fmt.Errorf("get transaction by id: %w", err)
	}
	return sqlcTransactionToDB(&row)
}

// List returns transactions matching the provided filter. Uses sqlc for the base
// user + pagination query when no extra filters are set; otherwise uses dynamic SQL.
func (r *PGXTransactionRepository) List(ctx context.Context, tx pgx.Tx, filter TransactionFilter) ([]Transaction, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	hasExtra := filter.AccountID != nil || filter.Type != nil ||
		filter.FromOccurredAt != nil || filter.ToOccurredAt != nil
	if !hasExtra {
		rows, err := r.q(tx).ListTransactionsByUser(ctx, sqlcdb.ListTransactionsByUserParams{
			UserID: uuidx.UUIDToPG(filter.UserID),
			Limit:  int32(filter.Limit),
			Offset: int32(filter.Offset),
		})
		if err != nil {
			return nil, fmt.Errorf("list transactions: %w", err)
		}
		out := make([]Transaction, 0, len(rows))
		for i := range rows {
			t, err := sqlcTransactionToDB(&rows[i])
			if err != nil {
				return nil, err
			}
			out = append(out, *t)
		}
		return out, nil
	}
	return r.listWithFilters(ctx, tx, filter)
}

// listWithFilters runs a dynamic query when optional filters are present.
func (r *PGXTransactionRepository) listWithFilters(ctx context.Context, tx pgx.Tx, filter TransactionFilter) ([]Transaction, error) {
	base := `
SELECT id, user_id, account_id, amount, currency, type, category_id, description,
       occurred_at, metadata, version, last_modified, source, sheets_row_id, deleted
FROM transactions
WHERE user_id = $1 AND deleted = false
`
	args := []any{filter.UserID}
	pos := 2
	if filter.AccountID != nil {
		base += fmt.Sprintf(" AND account_id = $%d", pos)
		args = append(args, *filter.AccountID)
		pos++
	}
	if filter.Type != nil {
		base += fmt.Sprintf(" AND type = $%d", pos)
		args = append(args, *filter.Type)
		pos++
	}
	if filter.FromOccurredAt != nil {
		base += fmt.Sprintf(" AND occurred_at >= $%d", pos)
		args = append(args, *filter.FromOccurredAt)
		pos++
	}
	if filter.ToOccurredAt != nil {
		base += fmt.Sprintf(" AND occurred_at <= $%d", pos)
		args = append(args, *filter.ToOccurredAt)
		pos++
	}
	base += " ORDER BY occurred_at DESC"
	base += fmt.Sprintf(" LIMIT $%d OFFSET $%d", pos, pos+1)
	args = append(args, filter.Limit, filter.Offset)

	var run func(context.Context, string, ...any) (pgx.Rows, error)
	if tx != nil {
		run = tx.Query
	} else {
		run = r.pool.Query
	}
	rows, err := run(ctx, base, args...)
	if err != nil {
		return nil, fmt.Errorf("query transactions: %w", err)
	}
	defer rows.Close()

	var result []Transaction
	for rows.Next() {
		var rec Transaction
		var rawMeta []byte
		var categoryID *uuid.UUID
		if err := rows.Scan(
			&rec.ID, &rec.UserID, &rec.AccountID, &rec.Amount, &rec.Currency, &rec.Type,
			&categoryID, &rec.Description, &rec.OccurredAt, &rawMeta,
			&rec.Version, &rec.LastModified, &rec.Source, &rec.SheetsRowID, &rec.Deleted,
		); err != nil {
			return nil, fmt.Errorf("scan transaction row: %w", err)
		}
		rec.CategoryID = categoryID
		if len(rawMeta) > 0 {
			if err := json.Unmarshal(rawMeta, &rec.Metadata); err != nil {
				return nil, fmt.Errorf("unmarshal metadata: %w", err)
			}
		}
		result = append(result, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transactions: %w", err)
	}
	return result, nil
}

// Update performs a full update of a transaction row and returns the updated record.
func (r *PGXTransactionRepository) Update(ctx context.Context, tx pgx.Tx, params UpdateTransactionParams) (*Transaction, error) {
	metaBytes, err := json.Marshal(params.Metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}

	amount, err := pgtypex.NumericFromFloat64(params.Amount)
	if err != nil {
		return nil, fmt.Errorf("convert amount: %w", err)
	}
	occurredAt, err := pgtypex.TimestamptzFromTime(params.OccurredAt)
	if err != nil {
		return nil, fmt.Errorf("convert occurred_at: %w", err)
	}
	description, err := pgtypex.TextFromStringPtr(params.Description)
	if err != nil {
		return nil, fmt.Errorf("convert description: %w", err)
	}
	sheetsRowID, err := pgtypex.TextFromStringPtr(params.SheetsRowID)
	if err != nil {
		return nil, fmt.Errorf("convert sheets_row_id: %w", err)
	}

	arg := sqlcdb.UpdateTransactionParams{
		ID:          uuidx.UUIDToPG(params.ID),
		UserID:      uuidx.UUIDToPG(params.UserID),
		AccountID:   uuidx.UUIDToPG(params.AccountID),
		Amount:      amount,
		Currency:    params.Currency,
		Type:        params.Type,
		CategoryID:  uuidx.UUIDPtrToPG(params.CategoryID),
		Description: description,
		OccurredAt:  occurredAt,
		Metadata:    metaBytes,
		Source:      params.Source,
		SheetsRowID: sheetsRowID,
	}
	row, err := r.q(tx).UpdateTransaction(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("update transaction: %w", err)
	}
	return sqlcTransactionToDB(&row)
}

// SoftDelete marks a transaction as deleted and returns the soft-deleted record.
func (r *PGXTransactionRepository) SoftDelete(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Transaction, error) {
	row, err := r.q(tx).SoftDeleteTransactionByID(ctx, uuidx.UUIDToPG(id))
	if err != nil {
		return nil, fmt.Errorf("soft delete transaction: %w", err)
	}
	return sqlcTransactionToDB(&row)
}

// sqlcTransactionToDB converts a sqlc Transaction (pgtype-based) to db.Transaction.
func sqlcTransactionToDB(t *sqlcdb.Transaction) (*Transaction, error) {
	id, err := uuidx.PGToUUID(t.ID)
	if err != nil {
		return nil, fmt.Errorf("convert id: %w", err)
	}
	userID, err := uuidx.PGToUUID(t.UserID)
	if err != nil {
		return nil, fmt.Errorf("convert user_id: %w", err)
	}
	accountID, err := uuidx.PGToUUID(t.AccountID)
	if err != nil {
		return nil, fmt.Errorf("convert account_id: %w", err)
	}

	amount, err := pgtypex.Float64FromNumeric(t.Amount)
	if err != nil {
		return nil, fmt.Errorf("convert amount: %w", err)
	}

	categoryID, err := uuidx.PGToUUIDPtr(t.CategoryID)
	if err != nil {
		return nil, fmt.Errorf("convert category_id: %w", err)
	}

	description := pgtypex.StringPtrFromText(t.Description)
	occurredAt := pgtypex.TimeFromTimestamptz(t.OccurredAt)
	lastModified := pgtypex.TimeFromTimestamptz(t.LastModified)
	sheetsRowID := pgtypex.StringPtrFromText(t.SheetsRowID)

	out := &Transaction{
		ID:           id,
		UserID:       userID,
		AccountID:    accountID,
		Amount:       amount,
		Currency:     t.Currency,
		Type:         t.Type,
		CategoryID:   categoryID,
		Description:  description,
		OccurredAt:   occurredAt,
		Version:      t.Version,
		LastModified: lastModified,
		Source:       t.Source,
		SheetsRowID:  sheetsRowID,
		Deleted:      t.Deleted,
	}

	if len(t.Metadata) > 0 {
		if err := json.Unmarshal(t.Metadata, &out.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}

	return out, nil
}

