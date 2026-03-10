package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"moon-eye/backend/pkg/shared/httpx"
)

type contextKey string

const (
	userIDKey contextKey = "userID"
	emailKey  contextKey = "email"
)

// Claims represents JWT claims required by backend services.
type Claims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// JWTMiddleware validates Bearer tokens and injects identity into the request context.
// secret should be provided via configuration (e.g. shared config.Auth.JWTSecret).
func JWTMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "missing Authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid Authorization header")
				return
			}

			tokenStr := parts[1]

			token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
				// TODO: support asymmetric keys if required by auth-service.
				return []byte(secret), nil
			})
			if err != nil || !token.Valid {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid token")
				return
			}

			claims, ok := token.Claims.(*Claims)
			if !ok {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid token claims")
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
			ctx = context.WithValue(ctx, emailKey, claims.Email)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFromContext extracts the user ID from request context.
func UserIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(userIDKey).(string); ok {
		return v
	}
	return ""
}

// EmailFromContext extracts the email from request context.
func EmailFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(emailKey).(string); ok {
		return v
	}
	return ""
}

