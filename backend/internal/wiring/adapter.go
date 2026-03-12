package wiring

import (
	"context"
	"time"

	"github.com/gofrs/uuid"
	"github.com/jackc/pgx/v5"

	"moon-eye/backend/internal/db"
	"moon-eye/backend/internal/domain"
	"moon-eye/backend/internal/service"
)

// ServiceTransactionAdapter adapts db.TransactionRepository to service.TransactionRepository.
type ServiceTransactionAdapter struct {
	repo db.TransactionRepository
	tx   pgx.Tx
}

// NewServiceTransactionAdapter returns a service.TransactionRepository. Pass nil for tx for pool-backed calls.
func NewServiceTransactionAdapter(repo db.TransactionRepository, tx pgx.Tx) service.TransactionRepository {
	return &ServiceTransactionAdapter{repo: repo, tx: tx}
}

func (a *ServiceTransactionAdapter) ListByUser(ctx context.Context, userID string, limit, offset int) ([]domain.Transaction, error) {
	uid, err := uuid.FromString(userID)
	if err != nil {
		return nil, err
	}
	filter := db.TransactionFilter{UserID: uid, Limit: limit, Offset: offset}
	rows, err := a.repo.List(ctx, a.tx, filter)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Transaction, 0, len(rows))
	for i := range rows {
		out = append(out, dbTransactionToDomain(&rows[i]))
	}
	return out, nil
}

func (a *ServiceTransactionAdapter) CountByUser(ctx context.Context, userID string) (int64, error) {
	uid, err := uuid.FromString(userID)
	if err != nil {
		return 0, err
	}
	filter := db.TransactionFilter{UserID: uid, Limit: 1, Offset: 0}
	rows, err := a.repo.List(ctx, a.tx, filter)
	if err != nil {
		return 0, err
	}
	return int64(len(rows)), nil
}

func (a *ServiceTransactionAdapter) Create(ctx context.Context, tx *domain.Transaction) error {
	params, err := domainTransactionToCreateParams(tx)
	if err != nil {
		return err
	}
	created, err := a.repo.Create(ctx, a.tx, params)
	if err != nil {
		return err
	}
	*tx = dbTransactionToDomain(created)
	return nil
}

func (a *ServiceTransactionAdapter) GetByID(ctx context.Context, userID, id string) (*domain.Transaction, error) {
	uid, err := uuid.FromString(id)
	if err != nil {
		return nil, err
	}
	row, err := a.repo.GetByID(ctx, a.tx, uid)
	if err != nil {
		return nil, err
	}
	d := dbTransactionToDomain(row)
	return &d, nil
}

func (a *ServiceTransactionAdapter) Update(ctx context.Context, tx *domain.Transaction) error {
	params, err := domainTransactionToUpdateParams(tx)
	if err != nil {
		return err
	}
	updated, err := a.repo.Update(ctx, a.tx, params)
	if err != nil {
		return err
	}
	*tx = dbTransactionToDomain(updated)
	return nil
}

func (a *ServiceTransactionAdapter) SoftDelete(ctx context.Context, userID, id string) (*domain.Transaction, error) {
	uid, err := uuid.FromString(id)
	if err != nil {
		return nil, err
	}
	row, err := a.repo.SoftDelete(ctx, a.tx, uid)
	if err != nil {
		return nil, err
	}
	d := dbTransactionToDomain(row)
	return &d, nil
}

func dbTransactionToDomain(r *db.Transaction) domain.Transaction {
	out := domain.Transaction{
		ID:           r.ID.String(),
		UserID:       r.UserID.String(),
		AccountID:    r.AccountID.String(),
		Amount:       r.Amount,
		Currency:     r.Currency,
		Type:         r.Type,
		OccurredAt:   r.OccurredAt,
		Version:      r.Version,
		LastModified: r.LastModified,
		Source:       r.Source,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
		DeletedAt:    r.DeletedAt,
	}
	if r.CategoryID != nil {
		s := r.CategoryID.String()
		out.CategoryID = &s
	}
	out.Description = r.Description
	if r.SheetsRowID != nil {
		out.SheetsRowID = r.SheetsRowID
	}
	out.Metadata = r.Metadata
	out.CreatedAt = r.CreatedAt
	out.UpdatedAt = r.UpdatedAt
	out.DeletedAt = r.DeletedAt
	return out
}

func domainTransactionToCreateParams(tx *domain.Transaction) (db.CreateTransactionParams, error) {
	userID, err := uuid.FromString(tx.UserID)
	if err != nil {
		return db.CreateTransactionParams{}, err
	}
	accountID, err := uuid.FromString(tx.AccountID)
	if err != nil {
		return db.CreateTransactionParams{}, err
	}
	var categoryID *uuid.UUID
	if tx.CategoryID != nil {
		cid, err := uuid.FromString(*tx.CategoryID)
		if err != nil {
			return db.CreateTransactionParams{}, err
		}
		categoryID = &cid
	}
	return db.CreateTransactionParams{
		UserID:       userID,
		AccountID:    accountID,
		Amount:       tx.Amount,
		Currency:     tx.Currency,
		Type:         tx.Type,
		CategoryID:   categoryID,
		Description:  tx.Description,
		OccurredAt:   tx.OccurredAt,
		Metadata:     tx.Metadata,
		Version:      tx.Version,
		LastModified: tx.LastModified,
		Source:       tx.Source,
		SheetsRowID:  tx.SheetsRowID,
		CreatedAt:    tx.OccurredAt,
		UpdatedAt:    tx.LastModified,
		DeletedAt:    time.Time{},
	}, nil
}

func domainTransactionToUpdateParams(tx *domain.Transaction) (db.UpdateTransactionParams, error) {
	id, err := uuid.FromString(tx.ID)
	if err != nil {
		return db.UpdateTransactionParams{}, err
	}
	userID, err := uuid.FromString(tx.UserID)
	if err != nil {
		return db.UpdateTransactionParams{}, err
	}
	accountID, err := uuid.FromString(tx.AccountID)
	if err != nil {
		return db.UpdateTransactionParams{}, err
	}
	var categoryID *uuid.UUID
	if tx.CategoryID != nil {
		cid, err := uuid.FromString(*tx.CategoryID)
		if err != nil {
			return db.UpdateTransactionParams{}, err
		}
		categoryID = &cid
	}
	return db.UpdateTransactionParams{
		ID:          id,
		UserID:      userID,
		AccountID:   accountID,
		Amount:      tx.Amount,
		Currency:    tx.Currency,
		Type:        tx.Type,
		CategoryID:  categoryID,
		Description: tx.Description,
		OccurredAt:  tx.OccurredAt,
		Metadata:    tx.Metadata,
		Source:      tx.Source,
		SheetsRowID: tx.SheetsRowID,
		Version:     tx.Version,
	}, nil
}
