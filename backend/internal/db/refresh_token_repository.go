package db

import (
	"context"
	"time"

	"github.com/gofrs/uuid"
	"github.com/jackc/pgx/v5"
)

//go:generate mockery --name=RefreshTokenRepository --output=internal/mocks --outpkg=mocks --case underscore

// RefreshTokenRepository defines the db-level contract for refresh token persistence.
// When tx is nil, implementations use the pool; when tx is non-nil, they use the transaction.
type RefreshTokenRepository interface {
	Create(ctx context.Context, tx pgx.Tx, token RefreshToken) error
	GetByID(ctx context.Context, id uuid.UUID) (*RefreshToken, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]RefreshToken, error)
	Revoke(ctx context.Context, tx pgx.Tx, id uuid.UUID) error
}

// RefreshToken matches the refresh_tokens table.
type RefreshToken struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	EncryptedToken string
	ExpiresAt      time.Time
	Revoked        bool
	CreatedAt      time.Time
}
