package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"moon-eye/backend/internal/service"
	"moon-eye/backend/pkg/shared/httpx"
)

// RegisterAuthHandlers mounts auth routes under the given router at /auth.
// So the caller should mount under /api/v1, e.g. r.Route("/api/v1", func(r chi.Router) { RegisterAuthHandlers(r, svc) })
// to get POST /api/v1/auth/register, /api/v1/auth/login, etc.
func RegisterAuthHandlers(r chi.Router, svc service.AuthService) {
	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", handleRegister(svc))
		r.Post("/login", handleLogin(svc))
		r.Post("/refresh", handleRefresh(svc))
		r.Post("/logout", handleLogout(svc))
	})
}

// --- Request/Response DTOs (camelCase JSON) ---

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type registerResponse struct {
	User userResponse `json:"user"`
}

type userResponse struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	CreatedAt string `json:"createdAt"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type refreshResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

type logoutRequest struct {
	RefreshToken string `json:"refreshToken"`
}

// --- Handlers ---

func handleRegister(svc service.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req registerRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid body")
			return
		}
		if req.Email == "" || req.Password == "" {
			httpx.WriteError(w, http.StatusBadRequest, "validation_error", "email and password required")
			return
		}
		user, err := svc.Register(r.Context(), req.Email, req.Password)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "registration_failed", err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, registerResponse{
			User: userToResponse(user),
		})
	}
}

func handleLogin(svc service.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid body")
			return
		}
		if req.Email == "" || req.Password == "" {
			httpx.WriteError(w, http.StatusBadRequest, "validation_error", "email and password required")
			return
		}
		accessToken, refreshToken, err := svc.Authenticate(r.Context(), req.Email, req.Password)
		if err != nil {
			if errors.Is(err, service.ErrInvalidCredentials) {
				httpx.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
				return
			}
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "authentication failed")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, loginResponse{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
		})
	}
}

func handleRefresh(svc service.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req refreshRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid body")
			return
		}
		if req.RefreshToken == "" {
			httpx.WriteError(w, http.StatusBadRequest, "validation_error", "refreshToken required")
			return
		}
		newAccess, newRefresh, err := svc.Refresh(r.Context(), req.RefreshToken)
		if err != nil {
			if errors.Is(err, service.ErrRefreshTokenInvalid) {
				httpx.WriteError(w, http.StatusUnauthorized, "invalid_refresh_token", "refresh token invalid or expired")
				return
			}
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "refresh failed")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, refreshResponse{
			AccessToken:  newAccess,
			RefreshToken: newRefresh,
		})
	}
}

func handleLogout(svc service.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req logoutRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid body")
			return
		}
		if req.RefreshToken == "" {
			httpx.WriteError(w, http.StatusBadRequest, "validation_error", "refreshToken required")
			return
		}
		err := svc.Revoke(r.Context(), req.RefreshToken)
		if err != nil {
			if errors.Is(err, service.ErrRefreshTokenInvalid) {
				httpx.WriteError(w, http.StatusBadRequest, "invalid_refresh_token", "refresh token invalid")
				return
			}
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "revoke failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func userToResponse(u service.User) userResponse {
	return userResponse{
		ID:        u.ID.String(),
		Email:     u.Email,
		CreatedAt: u.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
	}
}
