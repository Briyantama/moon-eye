-- Auth and refresh tokens (Phase E). Compatible with PostgreSQL 15+.
-- Does not modify transactions, change_events, or sheets_connections.

-- 1) users: add hashed_password for auth (table already exists from 0001)
-- Existing rows get empty string; backfill or require password set on first login.
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS hashed_password TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_users_email_lower ON users (LOWER(email));

-- 2) refresh_tokens: store encrypted refresh tokens (encrypt via pkg/shared/crypto before insert)
CREATE TABLE IF NOT EXISTS refresh_tokens (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  encrypted_token TEXT NOT NULL,
  expires_at      timestamptz NOT NULL,
  revoked         BOOLEAN NOT NULL DEFAULT false,
  created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens (user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires_at ON refresh_tokens (expires_at) WHERE revoked = false;

-- Down migration (run manually if needed):
-- DROP INDEX IF EXISTS idx_refresh_tokens_expires_at;
-- DROP INDEX IF EXISTS idx_refresh_tokens_user_id;
-- DROP TABLE IF EXISTS refresh_tokens;
-- DROP INDEX IF EXISTS idx_users_email_lower;
-- ALTER TABLE users DROP COLUMN IF EXISTS hashed_password;
