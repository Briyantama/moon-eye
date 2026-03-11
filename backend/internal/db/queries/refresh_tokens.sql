-- name: CreateRefreshToken :exec
INSERT INTO refresh_tokens (id, user_id, encrypted_token, expires_at, revoked, created_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetRefreshTokenByID :one
SELECT id, user_id, encrypted_token, expires_at, revoked, created_at
FROM refresh_tokens
WHERE id = $1;

-- name: GetRefreshTokensByUserID :many
SELECT id, user_id, encrypted_token, expires_at, revoked, created_at
FROM refresh_tokens
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens SET revoked = true WHERE id = $1;
