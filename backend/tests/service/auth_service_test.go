package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gofrs/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"moon-eye/backend/internal/db"
	"moon-eye/backend/internal/service"
)

// Manual test doubles for auth (db.UserRepository, db.RefreshTokenRepository, db.TxRunner).
// When mockery is configured for these interfaces, replace with mocks.

type authUserRepo struct {
	createErr   error
	getByEmail  *db.User
	getByEmailErr error
	getByID     *db.User
	getByIDErr  error
	updateErr   error
}

func (r *authUserRepo) Create(ctx context.Context, tx pgx.Tx, user db.User) error {
	return r.createErr
}
func (r *authUserRepo) GetByEmail(ctx context.Context, email string) (*db.User, error) {
	if r.getByEmailErr != nil {
		return nil, r.getByEmailErr
	}
	return r.getByEmail, nil
}
func (r *authUserRepo) GetByID(ctx context.Context, id uuid.UUID) (*db.User, error) {
	if r.getByIDErr != nil {
		return nil, r.getByIDErr
	}
	return r.getByID, nil
}
func (r *authUserRepo) Update(ctx context.Context, tx pgx.Tx, user db.User) error {
	return r.updateErr
}

type authTokenRepo struct {
	createErr  error
	getByID    *db.RefreshToken
	getByIDErr error
	revokeErr  error
}

func (r *authTokenRepo) Create(ctx context.Context, tx pgx.Tx, token db.RefreshToken) error {
	return r.createErr
}
func (r *authTokenRepo) GetByID(ctx context.Context, id uuid.UUID) (*db.RefreshToken, error) {
	if r.getByIDErr != nil {
		return nil, r.getByIDErr
	}
	return r.getByID, nil
}
func (r *authTokenRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]db.RefreshToken, error) {
	return nil, nil
}
func (r *authTokenRepo) Revoke(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	return r.revokeErr
}

type authTxRunner struct {
	runErr error
	runFn  func(ctx context.Context, tx pgx.Tx) error
}

func (r *authTxRunner) RunInTx(ctx context.Context, fn func(ctx context.Context, tx pgx.Tx) error) error {
	if r.runErr != nil {
		return r.runErr
	}
	return fn(ctx, nil) // no real tx in unit test
}

func TestAuthService_Register_Success(t *testing.T) {
	users := &authUserRepo{}
	tokens := &authTokenRepo{}
	txRunner := &authTxRunner{}
	svc := service.NewAuthService(users, tokens, txRunner)

	user, err := svc.Register(context.Background(), " uSer@example.com ", "secret123")
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, user.ID)
	require.Equal(t, "user@example.com", user.Email)
	require.False(t, user.CreatedAt.IsZero())
}

func TestAuthService_Register_Validation(t *testing.T) {
	svc := service.NewAuthService(&authUserRepo{}, &authTokenRepo{}, &authTxRunner{})

	_, err := svc.Register(context.Background(), "", "pass")
	require.Error(t, err)

	_, err = svc.Register(context.Background(), "a@b.com", "")
	require.Error(t, err)
}

func TestAuthService_Authenticate_InvalidCredentials(t *testing.T) {
	users := &authUserRepo{getByEmailErr: db.ErrNoRows}
	svc := service.NewAuthService(users, &authTokenRepo{}, &authTxRunner{})

	_, _, err := svc.Authenticate(context.Background(), "nobody@example.com", "wrong")
	require.Error(t, err)
	require.True(t, errors.Is(err, service.ErrInvalidCredentials))
}
