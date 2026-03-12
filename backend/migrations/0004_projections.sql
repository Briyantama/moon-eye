-- Read models and cursor for projection processors (eventual consistency).

-- Cursor per projector so each can resume from last processed event.
CREATE TABLE IF NOT EXISTS projection_cursors (
  projector_name TEXT PRIMARY KEY,
  last_event_id  BIGINT NOT NULL DEFAULT 0,
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now(),
  deleted_at     timestamptz NULL
);

-- Per-user, per-period transaction summary (e.g. monthly totals by type/currency).
CREATE TABLE IF NOT EXISTS transaction_summary (
  user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  period_key   TEXT NOT NULL,
  currency     CHAR(3) NOT NULL,
  type         TEXT NOT NULL,
  total_amount NUMERIC(18,2) NOT NULL DEFAULT 0,
  count        BIGINT NOT NULL DEFAULT 0,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  deleted_at   timestamptz NULL,
  PRIMARY KEY (user_id, period_key, currency, type)
);
CREATE INDEX IF NOT EXISTS idx_transaction_summary_user_period ON transaction_summary (user_id, period_key);

-- Per-user, per-account monthly balance snapshot (optional for reporting).
CREATE TABLE IF NOT EXISTS monthly_balance (
  user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  month_key  TEXT NOT NULL,
  balance    NUMERIC(18,2) NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz NULL,
  PRIMARY KEY (user_id, account_id, month_key)
);
CREATE INDEX IF NOT EXISTS idx_monthly_balance_user_month ON monthly_balance (user_id, month_key);
