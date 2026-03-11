package db

import (
	"github.com/jackc/pgx/v5/pgxpool"

	sqlcdb "moon-eye/backend/internal/db/sqlc"
)

// Repositories aggregates db-layer repositories for use by adapters and wiring.
type Repositories struct {
	Transactions   TransactionRepository
	ChangeEvents   ChangeEventRepository
	Users          UserRepository
	RefreshTokens  RefreshTokenRepository
}

// NewRepositories builds repository implementations from a pool and sqlc Queries.
func NewRepositories(pool *pgxpool.Pool, queries *sqlcdb.Queries) *Repositories {
	return &Repositories{
		Transactions:  NewTransactionRepository(pool, queries),
		ChangeEvents:  NewChangeEventRepository(pool, queries),
		Users:         NewUserRepository(pool, queries),
		RefreshTokens: NewRefreshTokenRepository(pool, queries),
	}
}
