package db

import (
	"context"
	"time"

	"github.com/gofrs/uuid"
	"github.com/jackc/pgx/v5"
)

//go:generate mockery --name=TransactionRepository --output=internal/mocks

// TransactionRepository defines the db-level contract for transaction persistence.
// It operates on concrete DB models and is designed to be used with or without
// an explicit pgx transaction. When tx is nil, implementations must use a
// connection pool; when tx is non-nil, they must execute within that transaction.
type TransactionRepository interface {
	Create(ctx context.Context, tx pgx.Tx, params CreateTransactionParams) (*Transaction, error)
	GetByID(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Transaction, error)
	List(ctx context.Context, tx pgx.Tx, filter TransactionFilter) ([]Transaction, error)
	Update(ctx context.Context, tx pgx.Tx, params UpdateTransactionParams) (*Transaction, error)
	SoftDelete(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Transaction, error)
}

// Transaction models the database representation of a transaction row.
// This is intentionally close to the schema, and can be mapped to the
// higher-level domain.Transaction type by callers.
type Transaction struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	AccountID    uuid.UUID
	Amount       float64
	Currency     string
	Type         string
	CategoryID   *uuid.UUID
	Description  *string
	OccurredAt   time.Time
	Metadata     map[string]any
	Version      int64
	LastModified time.Time
	Source       string
	SheetsRowID  *string
	Deleted      bool
}

// CreateTransactionParams captures the fields required to insert a new
// transaction row.
type CreateTransactionParams struct {
	UserID      uuid.UUID
	AccountID   uuid.UUID
	Amount      float64
	Currency    string
	Type        string
	CategoryID  *uuid.UUID
	Description *string
	OccurredAt  time.Time
	Metadata    map[string]any
	Version     int64
	LastModified time.Time
	Source      string
	SheetsRowID *string
	Deleted     bool
}

// UpdateTransactionParams captures updatable fields for an existing transaction.
// The ID and UserID pair uniquely identify the row to update.
type UpdateTransactionParams struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	AccountID   uuid.UUID
	Amount      float64
	Currency    string
	Type        string
	CategoryID  *uuid.UUID
	Description *string
	OccurredAt  time.Time
	Metadata    map[string]any
	Source      string
	SheetsRowID *string
}

// TransactionFilter provides a rich filter surface for listing transactions.
type TransactionFilter struct {
	UserID        uuid.UUID
	AccountID     *uuid.UUID
	Type          *string
	FromOccurredAt *time.Time
	ToOccurredAt   *time.Time
	Limit         int
	Offset        int
}

