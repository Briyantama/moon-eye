package service

import (
	"context"
	"encoding/json"
	"time"

	"moon-eye/backend/internal/domain"
)

// TransactionService implements application use cases around transactions.
// It depends only on interfaces, not on infrastructure concerns.
type TransactionService struct {
	repo     TransactionRepository
	events   ChangeEventWriter
	syncQ    SyncQueue
	txRunner TxManager
}

func NewTransactionService(repo TransactionRepository, events ChangeEventWriter, syncQ SyncQueue, txRunner TxManager) *TransactionService {
	return &TransactionService{
		repo:     repo,
		events:   events,
		syncQ:    syncQ,
		txRunner: txRunner,
	}
}

func (s *TransactionService) ListUserTransactions(ctx context.Context, userID string, limit, offset int) ([]domain.Transaction, error) {
	limit, offset = normalizePagination(limit, offset)
	return s.repo.ListByUser(ctx, userID, limit, offset)
}

type CreateTransactionInput struct {
	UserID      string
	AccountID   string
	Amount      float64
	Currency    string
	Type        string
	CategoryID  *string
	Description *string
	OccurredAt  time.Time
	Metadata    map[string]any
	Source      string
}

func (s *TransactionService) CreateTransaction(ctx context.Context, in CreateTransactionInput) (*domain.Transaction, error) {
	if in.Currency == "" {
		in.Currency = "IDR"
	}
	if in.Source == "" {
		in.Source = "app"
	}

	now := time.Now().UTC()
	tx := &domain.Transaction{
		UserID:       in.UserID,
		AccountID:    in.AccountID,
		Amount:       in.Amount,
		Currency:     in.Currency,
		Type:         in.Type,
		CategoryID:   in.CategoryID,
		Description:  in.Description,
		OccurredAt:   in.OccurredAt,
		Metadata:     in.Metadata,
		Version:      1,
		LastModified: now,
		Source:       in.Source,
		Deleted:      false,
	}

	if s.txRunner == nil {
		// Fallback to non-transactional behavior if no TxManager is configured.
		if err := s.repo.Create(ctx, tx); err != nil {
			return nil, err
		}
	} else {
		if err := s.txRunner.RunInTx(ctx, func(txCtx context.Context, uow TransactionUnitOfWork) error {
			if err := uow.Transactions().Create(txCtx, tx); err != nil {
				return err
			}

			if s.events != nil {
				payload, _ := json.Marshal(tx)
				if err := uow.ChangeEvents().Create(txCtx, ChangeEventInput{
					EntityType: "transaction",
					EntityID:   tx.ID,
					UserID:     tx.UserID,
					Operation:  "CREATE_TRANSACTION",
					Version:    tx.Version,
					Payload:    payload,
				}); err != nil {
					return err
				}
			}

			return nil
		}); err != nil {
			return nil, err
		}
	}

	// TODO: marshal transaction payload more efficiently if needed.
	payload, _ := json.Marshal(tx)

	if s.syncQ != nil {
		_ = s.syncQ.EnqueueSyncJob(ctx, SyncJob{
			Entity:    "transaction",
			Operation: "CREATE_TRANSACTION",
			Payload:   payload,
		})
	}

	return tx, nil
}

// PaginationResult holds pagination metadata for list operations.
type PaginationResult struct {
	Limit  int   // requested (and normalized) page size
	Offset int   // starting offset
	Total  int64 // total matching records
}

// ListUserTransactionsWithCount returns a page of transactions plus pagination metadata.
func (s *TransactionService) ListUserTransactionsWithCount(ctx context.Context, userID string, limit, offset int) ([]domain.Transaction, PaginationResult, error) {
	limit, offset = normalizePagination(limit, offset)

	items, err := s.repo.ListByUser(ctx, userID, limit, offset)
	if err != nil {
		return nil, PaginationResult{}, err
	}

	total, err := s.repo.CountByUser(ctx, userID)
	if err != nil {
		return nil, PaginationResult{}, err
	}

	return items, PaginationResult{
		Limit:  limit,
		Offset: offset,
		Total:  total,
	}, nil
}

// GetTransaction returns a single transaction for the given user.
func (s *TransactionService) GetTransaction(ctx context.Context, userID, id string) (*domain.Transaction, error) {
	return s.repo.GetByID(ctx, userID, id)
}

// UpdateTransactionInput captures fields that can be updated on a transaction.
type UpdateTransactionInput struct {
	AccountID   string
	Amount      float64
	Currency    string
	Type        string
	CategoryID  *string
	Description *string
	OccurredAt  time.Time
	Metadata    map[string]any
	Source      string
}

// UpdateTransaction performs a full update of a transaction and emits change
// events and sync jobs. Versioning and last_modified are handled in the
// repository / SQL layer.
func (s *TransactionService) UpdateTransaction(ctx context.Context, userID, id string, in UpdateTransactionInput) (*domain.Transaction, error) {
	// Load the existing transaction to enforce ownership and to provide a base
	// for updates.
	existing, err := s.repo.GetByID(ctx, userID, id)
	if err != nil {
		return nil, err
	}

	// Apply updates. Business rules (e.g. allowed type transitions) would live
	// here; for now we perform a straightforward overwrite of mutable fields.
	existing.AccountID = in.AccountID
	existing.Amount = in.Amount
	if in.Currency != "" {
		existing.Currency = in.Currency
	}
	if in.Type != "" {
		existing.Type = in.Type
	}
	existing.CategoryID = in.CategoryID
	existing.Description = in.Description
	existing.OccurredAt = in.OccurredAt
	existing.Metadata = in.Metadata
	if in.Source != "" {
		existing.Source = in.Source
	}
	existing.LastModified = time.Now().UTC()

	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, err
	}

	payload, _ := json.Marshal(existing)

	if s.events != nil {
		_ = s.events.Create(ctx, ChangeEventInput{
			EntityType: "transaction",
			EntityID:   existing.ID,
			UserID:     existing.UserID,
			Operation:  "update",
			Version:    existing.Version,
			Payload:    payload,
		})
	}

	if s.syncQ != nil {
		_ = s.syncQ.EnqueueSyncJob(ctx, SyncJob{
			Entity:    "transaction",
			Operation: "update",
			Payload:   payload,
		})
	}

	return existing, nil
}

// SoftDeleteTransaction marks a transaction as deleted and emits a delete
// change event and sync job.
func (s *TransactionService) SoftDeleteTransaction(ctx context.Context, userID, id string) (*domain.Transaction, error) {
	deleted, err := s.repo.SoftDelete(ctx, userID, id)
	if err != nil {
		return nil, err
	}

	payload, _ := json.Marshal(deleted)

	if s.events != nil {
		_ = s.events.Create(ctx, ChangeEventInput{
			EntityType: "transaction",
			EntityID:   deleted.ID,
			UserID:     deleted.UserID,
			Operation:  "delete",
			Version:    deleted.Version,
			Payload:    payload,
		})
	}

	if s.syncQ != nil {
		_ = s.syncQ.EnqueueSyncJob(ctx, SyncJob{
			Entity:    "transaction",
			Operation: "delete",
			Payload:   payload,
		})
	}

	return deleted, nil
}

// normalizePagination enforces sane defaults and bounds for pagination
// parameters.
func normalizePagination(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

