package domain

import "context"

// TransactionRepository defines the behavior required to persist and retrieve
// transactions, abstracting away the underlying data store.
type TransactionRepository interface {
	ListByUser(ctx context.Context, userID string, limit, offset int) ([]Transaction, error)
	Create(ctx context.Context, tx *Transaction) error
}

