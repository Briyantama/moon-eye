-- name: CreateUser :exec
INSERT INTO users (id, email, hashed_password, created_at, updated_at, deleted_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetUserByEmail :one
SELECT id, email, hashed_password, created_at, updated_at, deleted_at
FROM users
WHERE LOWER(email) = LOWER($1) AND deleted_at IS NULL;

-- name: GetUserByID :one
SELECT id, email, hashed_password, created_at, updated_at, deleted_at
FROM users
WHERE id = $1 AND deleted_at IS NULL;

-- name: UpdateUser :exec
UPDATE users
SET email = $2, hashed_password = $3, updated_at = $4, deleted_at = $5
WHERE id = $1 AND deleted_at IS NULL;
