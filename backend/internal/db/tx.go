package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TxRunner defines a helper for running functions inside a PostgreSQL transaction.
type TxRunner interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context, tx pgx.Tx) error) error
}

// PGXTxRunner implements TxRunner using a pgx connection pool.
type PGXTxRunner struct {
	pool *pgxpool.Pool
}

// NewTxRunner constructs a PGXTxRunner backed by the given pool.
func NewTxRunner(pool *pgxpool.Pool) TxRunner {
	return &PGXTxRunner{pool: pool}
}

// RunInTx begins a transaction, executes fn, and commits or rolls back based on the result.
func (r *PGXTxRunner) RunInTx(ctx context.Context, fn func(ctx context.Context, tx pgx.Tx) error) (err error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	if err = fn(ctx, tx); err != nil {
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return err
	}

	return nil
}

