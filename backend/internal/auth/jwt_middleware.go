package auth

import (
	"context"
	"net/http"
	"strings"

	sharedauth "moon-eye/backend/pkg/shared/auth"
	"moon-eye/backend/pkg/shared/httpx"
)

// JWTMiddleware validates Bearer tokens using pkg/shared/auth.Verify (JWT_SIGNING_KEY)
// and injects userID and email into the request context.
func JWTMiddleware() func(http.Handler) http.Handler {
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
			userID, email, err := sharedauth.Verify(parts[1])
			if err != nil {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid token")
				return
			}
			ctx := sharedauth.WithUserID(r.Context(), userID)
			ctx = sharedauth.WithEmail(ctx, email)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFromContext returns the user ID from context. Re-export from shared auth for convenience.
func UserIDFromContext(ctx context.Context) string {
	return sharedauth.UserIDFromContext(ctx)
}

// EmailFromContext returns the email from context. Re-export from shared auth for convenience.
func EmailFromContext(ctx context.Context) string {
	return sharedauth.EmailFromContext(ctx)
}

