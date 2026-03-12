-- Optimize transaction listing queries for pagination.

CREATE INDEX IF NOT EXISTS idx_transactions_user_deleted_occurred_at
ON transactions (user_id, deleted_at, occurred_at DESC);

