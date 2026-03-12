-- name: CreateRefreshToken :exec
INSERT INTO refresh_tokens (id, user_id, encrypted_token, expires_at, revoked, created_at, updated_at, deleted_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: GetRefreshTokenByID :one
SELECT id, user_id, encrypted_token, expires_at, revoked, created_at, updated_at, deleted_at
FROM refresh_tokens
WHERE id = $1;

-- name: GetRefreshTokensByUserID :many
SELECT id, user_id, encrypted_token, expires_at, revoked, created_at, updated_at, deleted_at
FROM refresh_tokens
WHERE user_id = $1
ORDER BY expires_at DESC, revoked, created_at DESC, id DESC;

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens SET revoked = true, updated_at = NOW() WHERE id = $1;
