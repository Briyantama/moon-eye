//go:build integration

package integration_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"moon-eye/backend/internal/db"
	sqlcdb "moon-eye/backend/internal/db/sqlc"
	service "moon-eye/backend/internal/service"
	sharedauth "moon-eye/backend/pkg/shared/auth"
)

// ensureAuthEnv sets JWT_SIGNING_KEY and APP_CRYPTO_KEY if unset so auth and crypto work in tests.
func ensureAuthEnv(t *testing.T) {
	t.Helper()
	if os.Getenv("JWT_SIGNING_KEY") == "" {
		os.Setenv("JWT_SIGNING_KEY", "integration-test-jwt-secret-at-least-32-bytes")
	}
	if os.Getenv("APP_CRYPTO_KEY") == "" {
		// 32 bytes hex = 64 chars
		os.Setenv("APP_CRYPTO_KEY", "0000000000000000000000000000000000000000000000000000000000000000")
	}
}

// TestAuth_Register_Login_Refresh_Revoke runs auth flows end-to-end against a real DB.
// Requires INTEGRATION_DB=1 and Docker for Postgres. JWT_SIGNING_KEY and APP_CRYPTO_KEY
// can be set externally or are set to test values by ensureAuthEnv.
func TestAuth_Register_Login_Refresh_Revoke(t *testing.T) {
	if os.Getenv("INTEGRATION_DB") == "" {
		t.Skip("integration test; set INTEGRATION_DB=1 to run")
	}
	ensureAuthEnv(t)

	_, dsn := setupPostgresContainer(t)
	pool := setupDatabase(t, dsn)
	runMigrations(t, pool)

	queries := sqlcdb.New(pool)
	repos := db.NewRepositories(pool, queries)
	txRunner := db.NewTxRunner(pool)
	authSvc := service.NewAuthService(repos.Users, repos.RefreshTokens, txRunner)

	ctx := context.Background()
	email, password := "auth-test@example.com", "secret123"

	// Register
	user, err := authSvc.Register(ctx, email, password)
	require.NoError(t, err)
	require.NotEmpty(t, user.ID)
	require.Equal(t, email, user.Email)
	require.False(t, user.CreatedAt.IsZero())

	// Login
	accessToken, refreshToken, err := authSvc.Authenticate(ctx, email, password)
	require.NoError(t, err)
	require.NotEmpty(t, accessToken)
	require.NotEmpty(t, refreshToken)

	// Verify JWT
	userID, userEmail, err := sharedauth.Verify(accessToken)
	require.NoError(t, err)
	require.Equal(t, user.ID.String(), userID)
	require.Equal(t, email, userEmail)

	// Refresh
	newAccess, newRefresh, err := authSvc.Refresh(ctx, refreshToken)
	require.NoError(t, err)
	require.NotEmpty(t, newAccess)
	require.NotEmpty(t, newRefresh)
	require.NotEqual(t, accessToken, newAccess)
	require.NotEqual(t, refreshToken, newRefresh)

	// Old refresh token should be invalid after rotation
	_, _, err = authSvc.Refresh(ctx, refreshToken)
	require.Error(t, err)
	require.ErrorIs(t, err, service.ErrRefreshTokenInvalid)

	// Revoke the new refresh token
	err = authSvc.Revoke(ctx, newRefresh)
	require.NoError(t, err)

	// Revoked token cannot be used to refresh
	_, _, err = authSvc.Refresh(ctx, newRefresh)
	require.Error(t, err)
	require.ErrorIs(t, err, service.ErrRefreshTokenInvalid)
}
