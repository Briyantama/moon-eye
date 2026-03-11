package service

import (
	"context"
	"encoding/json"
	"time"

	"moon-eye/backend/internal/domain"
	"moon-eye/backend/pkg/shared/pagination"
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
	limit, offset = pagination.Normalize(limit, offset)
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
					Operation:  "create",
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

	if s.syncQ != nil {
		payload := syncPayloadForUser(tx.UserID, 0) // version 1 just created; sync from 0
		_ = s.syncQ.EnqueueSyncJob(ctx, SyncJob{
			Entity:    "transaction",
			Operation: "create",
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
	limit, offset = pagination.Normalize(limit, offset)

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

	applyUpdates := func(t *domain.Transaction) {
		t.AccountID = in.AccountID
		t.Amount = in.Amount
		if in.Currency != "" {
			t.Currency = in.Currency
		}
		if in.Type != "" {
			t.Type = in.Type
		}
		t.CategoryID = in.CategoryID
		t.Description = in.Description
		t.OccurredAt = in.OccurredAt
		t.Metadata = in.Metadata
		if in.Source != "" {
			t.Source = in.Source
		}
		t.LastModified = time.Now().UTC()
	}

	if s.txRunner == nil {
		applyUpdates(existing)

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
				Payload:   syncPayloadForUser(existing.UserID, existing.Version-1),
			})
		}

		return existing, nil
	}

	// Transactional path: ensure DB update and change_event write happen in a
	// single transaction via TxManager / TransactionUnitOfWork.
	if err := s.txRunner.RunInTx(ctx, func(txCtx context.Context, uow TransactionUnitOfWork) error {
		applyUpdates(existing)

		if err := uow.Transactions().Update(txCtx, existing); err != nil {
			return err
		}

		if s.events != nil {
			payload, _ := json.Marshal(existing)
			if err := uow.ChangeEvents().Create(txCtx, ChangeEventInput{
				EntityType: "transaction",
				EntityID:   existing.ID,
				UserID:     existing.UserID,
				Operation:  "update",
				Version:    existing.Version,
				Payload:    payload,
			}); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return nil, err
	}

	if s.syncQ != nil {
		_ = s.syncQ.EnqueueSyncJob(ctx, SyncJob{
			Entity:    "transaction",
			Operation: "update",
			Payload:   syncPayloadForUser(existing.UserID, existing.Version-1),
		})
	}

	return existing, nil
}

// SoftDeleteTransaction marks a transaction as deleted and emits a delete
// change event and sync job.
func (s *TransactionService) SoftDeleteTransaction(ctx context.Context, userID, id string) (*domain.Transaction, error) {
	if s.txRunner == nil {
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
				Payload:   syncPayloadForUser(deleted.UserID, deleted.Version-1),
			})
		}

		return deleted, nil
	}

	var deleted *domain.Transaction

	if err := s.txRunner.RunInTx(ctx, func(txCtx context.Context, uow TransactionUnitOfWork) error {
		var err error
		deleted, err = uow.Transactions().SoftDelete(txCtx, userID, id)
		if err != nil {
			return err
		}

		if s.events != nil {
			payload, _ := json.Marshal(deleted)
			if err := uow.ChangeEvents().Create(txCtx, ChangeEventInput{
				EntityType: "transaction",
				EntityID:   deleted.ID,
				UserID:     deleted.UserID,
				Operation:  "delete",
				Version:    deleted.Version,
				Payload:    payload,
			}); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return nil, err
	}

	if s.syncQ != nil {
		_ = s.syncQ.EnqueueSyncJob(ctx, SyncJob{
			Entity:    "transaction",
			Operation: "delete",
			Payload:   syncPayloadForUser(deleted.UserID, deleted.Version-1),
		})
	}

	return deleted, nil
}

// syncPayloadForUser returns JSON payload for sync-service (userId + sinceVersion).
func syncPayloadForUser(userID string, sinceVersion int64) []byte {
	b, _ := json.Marshal(map[string]interface{}{
		"userId":       userID,
		"sinceVersion": sinceVersion,
	})
	return b
}
