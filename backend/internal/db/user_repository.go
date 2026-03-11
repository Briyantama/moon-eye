package db

import (
	"context"
	"time"

	"github.com/gofrs/uuid"
	"github.com/jackc/pgx/v5"
)

// ErrNoRows is returned when a query returns no rows. Use errors.Is(err, ErrNoRows).
var ErrNoRows = pgx.ErrNoRows

//go:generate mockery --name=UserRepository --output=internal/mocks --outpkg=mocks --case underscore

// UserRepository defines the db-level contract for user persistence.
// When tx is nil, implementations use the pool; when tx is non-nil, they use the transaction.
type UserRepository interface {
	Create(ctx context.Context, tx pgx.Tx, user User) error
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	Update(ctx context.Context, tx pgx.Tx, user User) error
}

// User matches the users table (id, email, hashed_password, created_at, updated_at, deleted).
type User struct {
	ID             uuid.UUID
	Email          string
	HashedPassword string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Deleted        bool
}
