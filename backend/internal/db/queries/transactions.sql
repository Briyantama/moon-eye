-- name: ListTransactionsByUser :many
-- List all non-deleted transactions for a user with pagination.
SELECT
  id,
  user_id,
  account_id,
  amount,
  currency,
  type,
  category_id,
  description,
  occurred_at,
  metadata,
  version,
  last_modified,
  source,
  sheets_row_id,
  created_at,
  updated_at,
  deleted_at
FROM transactions
WHERE user_id = $1
  AND deleted_at IS NULL
ORDER BY occurred_at DESC
LIMIT $2 OFFSET $3;

-- name: CreateTransaction :one
-- Create a new transaction for a user.
INSERT INTO transactions (
  user_id,
  account_id,
  amount,
  currency,
  type,
  category_id,
  description,
  occurred_at,
  metadata,
  version,
  last_modified,
  source,
  sheets_row_id,
  created_at,
  updated_at,
  deleted_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
)
RETURNING
  id,
  user_id,
  account_id,
  amount,
  currency,
  type,
  category_id,
  description,
  occurred_at,
  metadata,
  version,
  last_modified,
  source,
  sheets_row_id,
  created_at,
  updated_at,
  deleted_at;

-- name: CountTransactionsByUser :one
-- Count non-deleted transactions for a user.
SELECT COUNT(*)::bigint AS total
FROM transactions
WHERE user_id = $1
  AND deleted_at IS NULL;

-- name: GetTransactionByID :one
-- Fetch a single non-deleted transaction by id and user.
SELECT
  id,
  user_id,
  account_id,
  amount,
  currency,
  type,
  category_id,
  description,
  occurred_at,
  metadata,
  version,
  last_modified,
  source,
  sheets_row_id,
  created_at,
  updated_at,
  deleted_at
FROM transactions
WHERE id = $1
  AND user_id = $2
  AND deleted_at IS NULL;

-- name: GetTransactionByIDOnly :one
-- Fetch a single transaction by id (for use inside tx when user_id not needed).
SELECT
  id,
  user_id,
  account_id,
  amount,
  currency,
  type,
  category_id,
  description,
  occurred_at,
  metadata,
  version,
  last_modified,
  source,
  sheets_row_id,
  created_at,
  updated_at,
  deleted_at
FROM transactions
WHERE id = $1
  AND deleted_at IS NULL;

-- name: UpdateTransaction :one
-- Update an existing transaction and bump the version.
UPDATE transactions
SET
  account_id    = $3,
  amount        = $4,
  currency      = $5,
  type          = $6,
  category_id   = $7,
  description   = $8,
  occurred_at   = $9,
  metadata      = $10,
  last_modified = now(),
  source        = $11,
  sheets_row_id = $12,
  version       = version + 1,
  updated_at    = NOW()
WHERE id = $1
  AND user_id = $2
  AND version = $13
RETURNING
  id,
  user_id,
  account_id,
  amount,
  currency,
  type,
  category_id,
  description,
  occurred_at,
  metadata,
  version,
  last_modified,
  source,
  sheets_row_id,
  created_at,
  updated_at,
  deleted_at;

-- name: SoftDeleteTransaction :one
-- Soft-delete a transaction (id + user_id).
UPDATE transactions
SET
  deleted_at    = NOW(),
  last_modified = now(),
  version       = version + 1,
  updated_at    = NOW()
WHERE id = $1
  AND user_id = $2
  AND deleted_at IS NULL
RETURNING
  id,
  user_id,
  account_id,
  amount,
  currency,
  type,
  category_id,
  description,
  occurred_at,
  metadata,
  version,
  last_modified,
  source,
  sheets_row_id,
  created_at,
  updated_at,
  deleted_at;

-- name: SoftDeleteTransactionByID :one
-- Soft-delete a transaction by id only (for use when user_id not in scope).
UPDATE transactions
SET
  deleted_at    = NOW(),
  last_modified = now(),
  version       = version + 1,
  updated_at    = NOW()
WHERE id = $1
  AND deleted_at IS NULL
RETURNING
  id,
  user_id,
  account_id,
  amount,
  currency,
  type,
  category_id,
  description,
  occurred_at,
  metadata,
  version,
  last_modified,
  source,
  sheets_row_id,
  created_at,
  updated_at,
  deleted_at;

-- name: ListTransactionsByFilter :many
-- List transactions for a user with optional filters.
SELECT
  id,
  user_id,
  account_id,
  amount,
  currency,
  type,
  category_id,
  description,
  occurred_at,
  metadata,
  version,
  last_modified,
  source,
  sheets_row_id,
  created_at,
  updated_at,
  deleted_at
FROM transactions
WHERE user_id = $1
  AND deleted_at IS NULL
  AND ($2 = false OR account_id = $3)
  AND ($4 = false OR type = $5)
  AND ($6 = false OR occurred_at >= $7)
  AND ($8 = false OR occurred_at <= $9)
ORDER BY occurred_at DESC
LIMIT $10 OFFSET $11;
