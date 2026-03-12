package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/gofrs/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqlcdb "moon-eye/backend/internal/db/sqlc"
	"moon-eye/backend/pkg/shared/pgtypex"
	"moon-eye/backend/pkg/shared/uuidx"
)

// PGXUserRepository implements UserRepository using sqlc-generated queries.
type PGXUserRepository struct {
	pool *pgxpool.Pool
	q    *sqlcdb.Queries
}

// NewUserRepository returns a UserRepository. Uses sqlc Queries when provided; otherwise builds from pool.
func NewUserRepository(pool *pgxpool.Pool, q *sqlcdb.Queries) UserRepository {
	if q == nil {
		q = sqlcdb.New(pool)
	}
	return &PGXUserRepository{pool: pool, q: q}
}

func (r *PGXUserRepository) queries(tx pgx.Tx) *sqlcdb.Queries {
	if tx != nil {
		return r.q.WithTx(tx)
	}
	return r.q
}

func (r *PGXUserRepository) Create(ctx context.Context, tx pgx.Tx, user User) error {
	createdAt, err := pgtypex.TimestamptzFromTime(user.CreatedAt)
	if err != nil {
		return fmt.Errorf("created_at: %w", err)
	}
	updatedAt, err := pgtypex.TimestamptzFromTime(user.UpdatedAt)
	if err != nil {
		return fmt.Errorf("updated_at: %w", err)
	}
	q := r.queries(tx)
	var deletedAt pgtype.Timestamptz
	// for now we create users as non-deleted; DeletedAt stays NULL
	err = q.CreateUser(ctx, sqlcdb.CreateUserParams{
		ID:             uuidx.UUIDToPG(user.ID),
		Email:          user.Email,
		HashedPassword: user.HashedPassword,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
		DeletedAt:      deletedAt,
	})
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (r *PGXUserRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
	row, err := r.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoRows
		}
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return sqlcRowToUser(row.ID, row.Email, row.HashedPassword, row.CreatedAt, row.UpdatedAt, row.DeletedAt), nil
}

func (r *PGXUserRepository) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	row, err := r.q.GetUserByID(ctx, uuidx.UUIDToPG(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoRows
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return sqlcRowToUser(row.ID, row.Email, row.HashedPassword, row.CreatedAt, row.UpdatedAt, row.DeletedAt), nil
}

func (r *PGXUserRepository) Update(ctx context.Context, tx pgx.Tx, user User) error {
	updatedAt, err := pgtypex.TimestamptzFromTime(user.UpdatedAt)
	if err != nil {
		return fmt.Errorf("updated_at: %w", err)
	}
	q := r.queries(tx)
	var deletedAt pgtype.Timestamptz
	if user.Deleted {
		// mark as soft-deleted using current time
		if ts, convErr := pgtypex.TimestamptzFromTime(user.UpdatedAt); convErr == nil {
			deletedAt = ts
		}
	}
	err = q.UpdateUser(ctx, sqlcdb.UpdateUserParams{
		ID:             uuidx.UUIDToPG(user.ID),
		Email:          user.Email,
		HashedPassword: user.HashedPassword,
		UpdatedAt:      updatedAt,
		DeletedAt:      deletedAt,
	})
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}

func sqlcRowToUser(id pgtype.UUID, email, hashedPassword string, createdAt, updatedAt, deletedAt pgtype.Timestamptz) *User {
	u := &User{
		Email:          email,
		HashedPassword: hashedPassword,
		Deleted:        deletedAt.Valid,
	}
	u.ID, _ = uuidx.PGToUUID(id)
	u.CreatedAt = pgtypex.TimeFromTimestamptz(createdAt)
	u.UpdatedAt = pgtypex.TimeFromTimestamptz(updatedAt)
	return u
}
