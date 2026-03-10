package service

import (
	"context"

	"moon-eye/backend/internal/domain"
)

// TransactionRepository abstracts persistence for transactions.
// It is intentionally richer than the domain-level repository so the service
// layer can express all required use cases (CRUD + pagination).
type TransactionRepository interface {
	ListByUser(ctx context.Context, userID string, limit, offset int) ([]domain.Transaction, error)
	CountByUser(ctx context.Context, userID string) (int64, error)
	Create(ctx context.Context, tx *domain.Transaction) error
	GetByID(ctx context.Context, userID, id string) (*domain.Transaction, error)
	Update(ctx context.Context, tx *domain.Transaction) error
	SoftDelete(ctx context.Context, userID, id string) (*domain.Transaction, error)
}

// TxManager orchestrates database transactions for multi-write operations.
type TxManager interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context, uow TransactionUnitOfWork) error) error
}

// TransactionUnitOfWork exposes tx-scoped repositories used within a single transaction.
type TransactionUnitOfWork interface {
	Transactions() TransactionRepository
	ChangeEvents() ChangeEventWriter
}

// ChangeEventWriter abstracts writing change events.
type ChangeEventWriter interface {
	Create(ctx context.Context, e ChangeEventInput) error
}

// SyncQueue abstracts publishing sync jobs for background workers.
type SyncQueue interface {
	EnqueueSyncJob(ctx context.Context, job SyncJob) error
}

// ChangeEventInput represents data required to emit a change event from services.
type ChangeEventInput struct {
	EntityType string
	EntityID   string
	UserID     string
	Operation  string
	Version    int64
	Payload    []byte
}

// SyncJob is a generic sync queue job payload.
type SyncJob struct {
	Entity    string
	Operation string
	Payload   []byte
}

