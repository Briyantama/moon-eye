package wiring

import (
	"context"

	"github.com/jackc/pgx/v5"

	"moon-eye/backend/internal/db"
	"moon-eye/backend/internal/service"
)

// TxManagerAdapter adapts db.TxRunner to service.TxManager.
type TxManagerAdapter struct {
	runner  db.TxRunner
	txRepo  db.TransactionRepository
	evtRepo db.ChangeEventRepository
}

// NewTxManagerAdapter returns a service.TxManager that uses the given TxRunner and repos.
func NewTxManagerAdapter(runner db.TxRunner, txRepo db.TransactionRepository, evtRepo db.ChangeEventRepository) service.TxManager {
	return &TxManagerAdapter{runner: runner, txRepo: txRepo, evtRepo: evtRepo}
}

// RunInTx implements service.TxManager.
func (a *TxManagerAdapter) RunInTx(ctx context.Context, fn func(ctx context.Context, uow service.TransactionUnitOfWork) error) error {
	return a.runner.RunInTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		uow := &txUnitOfWork{
			txRepo:  NewServiceTransactionAdapter(a.txRepo, tx),
			evtRepo: &txChangeEventWriter{repo: a.evtRepo, tx: tx},
		}
		return fn(ctx, uow)
	})
}

type txUnitOfWork struct {
	txRepo  service.TransactionRepository
	evtRepo service.ChangeEventWriter
}

func (u *txUnitOfWork) Transactions() service.TransactionRepository { return u.txRepo }
func (u *txUnitOfWork) ChangeEvents() service.ChangeEventWriter       { return u.evtRepo }

type txChangeEventWriter struct {
	repo db.ChangeEventRepository
	tx   pgx.Tx
}

func (w *txChangeEventWriter) Create(ctx context.Context, e service.ChangeEventInput) error {
	return w.repo.Insert(ctx, w.tx, db.ChangeEvent{
		EntityType: e.EntityType,
		EntityID:   e.EntityID,
		UserID:     e.UserID,
		OpType:     e.Operation,
		Payload:    e.Payload,
		Version:    e.Version,
	})
}
