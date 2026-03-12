package db

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gofrs/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
	createdAt, err := pgtypex.TimestamptzFromTime(params.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("convert created_at: %w", err)
	}
	updatedAt, err := pgtypex.TimestamptzFromTime(params.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("convert updated_at: %w", err)
	}
	deletedAt, err := pgtypex.TimestamptzFromTime(params.DeletedAt)
	if err != nil {
		return nil, fmt.Errorf("convert deleted_at: %w", err)
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
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
		DeletedAt:    deletedAt,
	}
	row, err := r.q(tx).CreateTransaction(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("create transaction: %w", err)
	}
	return sqlcTransactionToDB(&sqlcdb.Transaction{
		ID:           row.ID,
		UserID:       row.UserID,
		AccountID:    row.AccountID,
		Amount:       row.Amount,
		Currency:     row.Currency,
		Type:         row.Type,
		CategoryID:   row.CategoryID,
		Description:  row.Description,
		OccurredAt:   row.OccurredAt,
		Metadata:     row.Metadata,
		Version:      row.Version,
		LastModified: row.LastModified,
		Source:       row.Source,
		SheetsRowID:  row.SheetsRowID,
		DeletedAt:    row.DeletedAt,
	})
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
	// Use a single sqlc query with optional filters instead of dynamic SQL.
	rows, err := r.q(tx).ListTransactionsByFilter(ctx, toListFilterParams(filter))
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

// toListFilterParams converts a TransactionFilter into sqlc ListTransactionsByFilterParams.
func toListFilterParams(filter TransactionFilter) sqlcdb.ListTransactionsByFilterParams {
	var (
		hasAccount   bool
		hasType      bool
		hasFrom      bool
		hasTo        bool
		accountID    pgtype.UUID
		typ          string
		fromOccurred pgtype.Timestamptz
		toOccurred   pgtype.Timestamptz
	)

	if filter.AccountID != nil {
		hasAccount = true
		accountID = uuidx.UUIDToPG(*filter.AccountID)
	}
	if filter.Type != nil {
		hasType = true
		typ = *filter.Type
	}
	if filter.FromOccurredAt != nil {
		hasFrom = true
		fromOccurred, _ = pgtypex.TimestamptzFromTime(*filter.FromOccurredAt)
	}
	if filter.ToOccurredAt != nil {
		hasTo = true
		toOccurred, _ = pgtypex.TimestamptzFromTime(*filter.ToOccurredAt)
	}

	return sqlcdb.ListTransactionsByFilterParams{
		UserID:       uuidx.UUIDToPG(filter.UserID),
		Column2:      hasAccount,
		AccountID:    accountID,
		Column4:      hasType,
		Type:         typ,
		Column6:      hasFrom,
		OccurredAt:   fromOccurred,
		Column8:      hasTo,
		OccurredAt_2: toOccurred,
		Limit:        int32(filter.Limit),
		Offset:       int32(filter.Offset),
	}
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
		Version:     params.Version,
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
	createdAt := pgtypex.TimeFromTimestamptz(t.CreatedAt)
	updatedAt := pgtypex.TimeFromTimestamptz(t.UpdatedAt)
	deletedAt := pgtypex.TimeFromTimestamptz(t.DeletedAt)
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
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
		DeletedAt:    deletedAt,
	}

	if len(t.Metadata) > 0 {
		if err := json.Unmarshal(t.Metadata, &out.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}

	return out, nil
}

