package db

import (
	"github.com/jackc/pgx/v5/pgxpool"

	sqlcdb "moon-eye/backend/internal/db/sqlc"
)

// Queries is an alias to the sqlc-generated Queries type so callers can depend
// on db.Queries without importing the generated package directly.
type Queries = sqlcdb.Queries

// Repositories aggregates all database-backed repositories used by services.
// It centralizes construction and wiring while keeping each repository modular
// and interface-driven.
type Repositories struct {
	Transactions TransactionRepository
	ChangeEvents ChangeEventRepository
}

// NewRepositories constructs all repository implementations backed by the given
// connection pool and sqlc Queries instance.
func NewRepositories(pool *pgxpool.Pool, queries *Queries) *Repositories {
	return &Repositories{
		Transactions: NewTransactionRepository(pool, queries),
		ChangeEvents: NewChangeEventRepository(pool, queries),
	}
}

