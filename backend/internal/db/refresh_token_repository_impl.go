package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/gofrs/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	sqlcdb "moon-eye/backend/internal/db/sqlc"
	"moon-eye/backend/pkg/shared/pgtypex"
	"moon-eye/backend/pkg/shared/uuidx"
)

// PGXRefreshTokenRepository implements RefreshTokenRepository using sqlc-generated queries.
type PGXRefreshTokenRepository struct {
	pool *pgxpool.Pool
	q    *sqlcdb.Queries
}

// NewRefreshTokenRepository returns a RefreshTokenRepository. Uses sqlc Queries when provided; otherwise builds from pool.
func NewRefreshTokenRepository(pool *pgxpool.Pool, q *sqlcdb.Queries) RefreshTokenRepository {
	if q == nil {
		q = sqlcdb.New(pool)
	}
	return &PGXRefreshTokenRepository{pool: pool, q: q}
}

func (r *PGXRefreshTokenRepository) queries(tx pgx.Tx) *sqlcdb.Queries {
	if tx != nil {
		return r.q.WithTx(tx)
	}
	return r.q
}

func (r *PGXRefreshTokenRepository) Create(ctx context.Context, tx pgx.Tx, token RefreshToken) error {
	expiresAt, err := pgtypex.TimestamptzFromTime(token.ExpiresAt)
	if err != nil {
		return fmt.Errorf("expires_at: %w", err)
	}
	createdAt, err := pgtypex.TimestamptzFromTime(token.CreatedAt)
	if err != nil {
		return fmt.Errorf("created_at: %w", err)
	}
	q := r.queries(tx)
	err = q.CreateRefreshToken(ctx, sqlcdb.CreateRefreshTokenParams{
		ID:             uuidx.UUIDToPG(token.ID),
		UserID:         uuidx.UUIDToPG(token.UserID),
		EncryptedToken: token.EncryptedToken,
		ExpiresAt:      expiresAt,
		Revoked:        token.Revoked,
		CreatedAt:      createdAt,
	})
	if err != nil {
		return fmt.Errorf("create refresh token: %w", err)
	}
	return nil
}

func (r *PGXRefreshTokenRepository) GetByID(ctx context.Context, id uuid.UUID) (*RefreshToken, error) {
	row, err := r.q.GetRefreshTokenByID(ctx, uuidx.UUIDToPG(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoRows
		}
		return nil, fmt.Errorf("get refresh token by id: %w", err)
	}
	return sqlcRefreshTokenToToken(&row), nil
}

func (r *PGXRefreshTokenRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]RefreshToken, error) {
	rows, err := r.q.GetRefreshTokensByUserID(ctx, uuidx.UUIDToPG(userID))
	if err != nil {
		return nil, fmt.Errorf("get refresh tokens by user: %w", err)
	}
	out := make([]RefreshToken, 0, len(rows))
	for i := range rows {
		out = append(out, *sqlcRefreshTokenToToken(&rows[i]))
	}
	return out, nil
}

func (r *PGXRefreshTokenRepository) Revoke(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	q := r.queries(tx)
	err := q.RevokeRefreshToken(ctx, uuidx.UUIDToPG(id))
	if err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}

func sqlcRefreshTokenToToken(row *sqlcdb.RefreshToken) *RefreshToken {
	t := &RefreshToken{
		EncryptedToken: row.EncryptedToken,
		Revoked:        row.Revoked,
	}
	t.ID, _ = uuidx.PGToUUID(row.ID)
	t.UserID, _ = uuidx.PGToUUID(row.UserID)
	t.ExpiresAt = pgtypex.TimeFromTimestamptz(row.ExpiresAt)
	t.CreatedAt = pgtypex.TimeFromTimestamptz(row.CreatedAt)
	return t
}
