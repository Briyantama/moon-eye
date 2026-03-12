-- Initial schema for moon-eye finance app

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- 1) users
CREATE TABLE IF NOT EXISTS users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email TEXT UNIQUE NOT NULL,
  display_name TEXT,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz NULL
);
CREATE INDEX IF NOT EXISTS idx_users_email ON users (email, deleted_at);

-- 2) accounts
CREATE TABLE IF NOT EXISTS accounts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  currency CHAR(3) NOT NULL DEFAULT 'IDR',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz NULL
);
CREATE INDEX IF NOT EXISTS idx_accounts_user_id ON accounts (user_id, deleted_at);

-- 3) categories
CREATE TABLE IF NOT EXISTS categories (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  is_income BOOLEAN NOT NULL DEFAULT false,
  parent_id UUID NULL REFERENCES categories(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz NULL
);
CREATE INDEX IF NOT EXISTS idx_categories_user_id ON categories (user_id, deleted_at);

-- 4) transactions
CREATE TABLE IF NOT EXISTS transactions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  account_id UUID NOT NULL REFERENCES accounts(id),
  amount NUMERIC(18,2) NOT NULL,
  currency CHAR(3) NOT NULL DEFAULT 'IDR',
  type TEXT NOT NULL CHECK (type IN ('expense','income','transfer')),
  category_id UUID REFERENCES categories(id),
  description TEXT,
  occurred_at timestamptz NOT NULL,
  metadata JSONB DEFAULT '{}'::jsonb,
  version BIGINT NOT NULL DEFAULT 1,
  last_modified timestamptz NOT NULL DEFAULT now(),
  source TEXT NOT NULL DEFAULT 'app',
  sheets_row_id TEXT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz NULL
);
CREATE INDEX IF NOT EXISTS idx_transactions_user_account ON transactions (user_id, account_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_transactions_user_last_modified ON transactions (user_id, last_modified DESC);

-- 5) change_events
CREATE TABLE IF NOT EXISTS change_events (
  id BIGSERIAL PRIMARY KEY,
  entity_type TEXT NOT NULL,
  entity_id UUID NOT NULL,
  user_id UUID NOT NULL,
  op_type TEXT NOT NULL CHECK (op_type IN ('create','update','delete')),
  payload JSONB NOT NULL,
  version BIGINT NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz NULL
);
CREATE INDEX IF NOT EXISTS idx_change_events_user_entity ON change_events (user_id, entity_type, entity_id, deleted_at);

-- 6) sheets_connections
CREATE TABLE IF NOT EXISTS sheets_connections (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  google_user_id TEXT NOT NULL,
  access_token TEXT NOT NULL,
  refresh_token TEXT,
  token_expiry timestamptz,
  sheet_id TEXT NOT NULL,
  sheet_range TEXT,
  sync_mode TEXT NOT NULL DEFAULT 'two-way',
  last_synced_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz NULL
);
CREATE INDEX IF NOT EXISTS idx_sheets_connections_user_id ON sheets_connections (user_id, deleted_at);

-- 7) sheet_mappings
CREATE TABLE IF NOT EXISTS sheet_mappings (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  connection_id UUID NOT NULL REFERENCES sheets_connections(id) ON DELETE CASCADE,
  sheet_column TEXT NOT NULL,
  db_field TEXT NOT NULL,
  transform JSONB,
  version BIGINT NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz NULL
);
CREATE INDEX IF NOT EXISTS idx_sheet_mappings_connection_id ON sheet_mappings (connection_id, deleted_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sheet_mappings_sheet_column_connection_id ON sheet_mappings (sheet_column, connection_id, deleted_at);

-- 8) sync_queue
CREATE TABLE IF NOT EXISTS sync_queue (
  id BIGSERIAL PRIMARY KEY,
  user_id UUID NOT NULL,
  op_type TEXT NOT NULL,
  payload JSONB NOT NULL,
  attempts INT NOT NULL DEFAULT 0,
  available_at timestamptz NOT NULL DEFAULT now(),
  max_attempts INT NOT NULL DEFAULT 10,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz NULL
);
CREATE INDEX IF NOT EXISTS idx_sync_queue_available_at ON sync_queue (available_at, deleted_at);

