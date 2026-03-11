package auth

import "context"

type contextKey string

const (
	userIDContextKey contextKey = "userID"
	emailContextKey  contextKey = "email"
)

// WithUserID returns a context with the user ID set. Use from auth middleware.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDContextKey, userID)
}

// WithEmail returns a context with the email set. Use from auth middleware.
func WithEmail(ctx context.Context, email string) context.Context {
	return context.WithValue(ctx, emailContextKey, email)
}

// UserIDFromContext returns the user ID from the request context, or empty string if not set.
func UserIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(userIDContextKey).(string); ok {
		return v
	}
	return ""
}

// EmailFromContext returns the email from the request context, or empty string if not set.
func EmailFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(emailContextKey).(string); ok {
		return v
	}
	return ""
}
