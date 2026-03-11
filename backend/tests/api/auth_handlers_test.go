package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"moon-eye/backend/internal/api"
	"moon-eye/backend/internal/service"
)

// mockAuthService is a minimal stub for handler tests.
type mockAuthService struct {
	registerErr     error
	registerUser    service.User
	authenticateErr error
	accessToken     string
	refreshToken    string
	refreshErr      error
	revokeErr       error
}

func (m *mockAuthService) Register(ctx context.Context, email, password string) (service.User, error) {
	if m.registerErr != nil {
		return service.User{}, m.registerErr
	}
	return m.registerUser, nil
}
func (m *mockAuthService) Authenticate(ctx context.Context, email, password string) (accessToken, refreshToken string, err error) {
	if m.authenticateErr != nil {
		return "", "", m.authenticateErr
	}
	return m.accessToken, m.refreshToken, nil
}
func (m *mockAuthService) Refresh(ctx context.Context, refreshToken string) (newAccessToken, newRefreshToken string, err error) {
	if m.refreshErr != nil {
		return "", "", m.refreshErr
	}
	return "new_access", "new_refresh", nil
}
func (m *mockAuthService) Revoke(ctx context.Context, refreshToken string) error {
	return m.revokeErr
}

var _ service.AuthService = (*mockAuthService)(nil)

func TestAuthHandlers_Register_ValidationError(t *testing.T) {
	svc := &mockAuthService{}
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		api.RegisterAuthHandlers(r, svc)
	})

	body := bytes.NewBufferString(`{"email":"","password":"x"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var out map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
	require.Contains(t, out, "error")
}

func TestAuthHandlers_Login_ValidationError(t *testing.T) {
	svc := &mockAuthService{}
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		api.RegisterAuthHandlers(r, svc)
	})

	body := bytes.NewBufferString(`{"email":"a@b.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAuthHandlers_Logout_Success(t *testing.T) {
	svc := &mockAuthService{}
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		api.RegisterAuthHandlers(r, svc)
	})

	body := bytes.NewBufferString(`{"refreshToken":"some-token"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
}
