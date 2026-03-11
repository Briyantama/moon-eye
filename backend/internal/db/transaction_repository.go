package db

import (
	"context"

	"github.com/gofrs/uuid"
	"github.com/jackc/pgx/v5"
)

//go:generate mockery --name=TransactionRepository --output=internal/mocks --outpkg=mocks --case underscore

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

