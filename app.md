# System architecture (production-grade)

**Summary architecture**

This architecture is designed for scalability, observability, and reliability — using separate components for API, sync workers, queue, and storage. It supports multi-tenant (per-user Google Sheets OAuth), offline-first on mobile, and retry-safe sync.

```
[Flutter Mobile]      [Next.js Web]
      \                   /
       \---> [API Gateway / Load Balancer] ---> [Go REST API (stateless)]
                             |
                  -------------------------
                 |                         |
            [Sync Worker(s)]           [Background Jobs]
                 |                         |
             [Queue: Redis/CloudPubSub]   |
                 |                         |
              [Postgres (Primary)] <---- [Blob store (R2 / GCS)]
                 |
            [Metrics, Logs, Traces]
```

**Main components**
- API: stateless Golang REST API (containerized). Autoscale based on CPU/requests.
- Auth: Google OAuth2 (for Sheets) + JWT session tokens.
- DB: managed PostgreSQL (primary); read replicas optional.
- Queue: Redis streams or Cloud Pub/Sub for reliable at-least-once processing.
- Sync Workers: separate worker pool for two-way synchronization with Google Sheets.
- Blob store: for attachments/exports (Cloudflare R2 / Google Cloud Storage).
- Observability: Prometheus metrics + Grafana, OpenTelemetry traces, structured logs.
- CI/CD: GitHub Actions → build → container image → registry → deploy (Render/Fly/Vercel).

## Recommended providers (free-tier friendly)

- Render (cloud hosting)
- Fly.io (edge app platform)
- Supabase (managed Postgres)
- Vercel (Next.js hosting)
- Google Cloud (cloud provider)
- Cloudflare (edge network provider)

---

## Database schema (Postgres) — finance app specific

This design focuses on auditability, conflict resolution, and efficient reporting.

## Main tables (summary)

1. `users` — user accounts
2. `accounts` — wallets / user accounts (multiple per user)
3. `categories` — transaction categories
4. `transactions` — canonical transaction records
5. `transaction_line_items` — optional for split transactions
6. `sheets_connections` — Google Sheets connections per user
7. `sheet_mappings` — mapping between sheet columns and DB fields
8. `change_events` — immutable change log for sync / audit
9. `sync_queue` — queue of operations for workers (idempotent)
10. `devices` — device metadata & last_sync cursors

## Detailed tables (columns & indexes — example SQL)

### 1) `users`

```sql
CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email TEXT UNIQUE NOT NULL,
  display_name TEXT,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON users (email);
```

### 2) `accounts`

```sql
CREATE TABLE accounts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  currency CHAR(3) NOT NULL DEFAULT 'IDR',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON accounts (user_id);
```

### 3) `categories`

```sql
CREATE TABLE categories (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  is_income BOOLEAN NOT NULL DEFAULT false,
  parent_id UUID NULL REFERENCES categories(id),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON categories (user_id);
```

### 4) `transactions` (core)

**Important:** each transaction has `version` and `last_modified` for conflict resolution. `source` indicates origin (mobile/web/sheets).

```sql
CREATE TABLE transactions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  account_id UUID NOT NULL REFERENCES accounts(id),
  amount NUMERIC(18,2) NOT NULL,
  currency CHAR(3) NOT NULL DEFAULT 'IDR',
  type TEXT NOT NULL CHECK (type IN ('expense','income','transfer')),
  category_id UUID REFERENCES categories(id),
  description TEXT,
  occurred_at timestamptz NOT NULL,
  metadata JSONB DEFAULT '{}',
  version BIGINT NOT NULL DEFAULT 1,
  last_modified timestamptz NOT NULL DEFAULT now(),
  source TEXT NOT NULL DEFAULT 'app',
  sheets_row_id TEXT NULL,
  deleted BOOLEAN NOT NULL DEFAULT false
);
CREATE INDEX ON transactions (user_id, account_id);
CREATE INDEX ON transactions (user_id, last_modified DESC);
```

### 5) `change_events` (immutable audit log)

```sql
CREATE TABLE change_events (
  id BIGSERIAL PRIMARY KEY,
  entity_type TEXT NOT NULL,
  entity_id UUID NOT NULL,
  user_id UUID NOT NULL,
  op_type TEXT NOT NULL CHECK (op_type IN ('create','update','delete')),
  payload JSONB NOT NULL,
  version BIGINT NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON change_events (user_id, entity_type, entity_id);
```

### 6) `sheets_connections`

```sql
CREATE TABLE sheets_connections (
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
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON sheets_connections (user_id);
```

### 7) `sheet_mappings`

```sql
CREATE TABLE sheet_mappings (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  connection_id UUID NOT NULL REFERENCES sheets_connections(id) ON DELETE CASCADE,
  sheet_column TEXT NOT NULL,
  db_field TEXT NOT NULL,
  transform JSONB,
  UNIQUE (sheet_column, connection_id)
);
```

### 8) `sync_queue`

```sql
CREATE TABLE sync_queue (
  id BIGSERIAL PRIMARY KEY,
  user_id UUID NOT NULL,
  op_type TEXT NOT NULL,
  payload JSONB NOT NULL,
  attempts INT NOT NULL DEFAULT 0,
  available_at timestamptz NOT NULL DEFAULT now(),
  max_attempts INT NOT NULL DEFAULT 10,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON sync_queue (available_at);
```

---

# Google Sheets — two-way sync design (algorithm & pseudocode)

Goal: two-way sync that is **idempotent**, resilient to partial failures, and deterministic in conflict resolution.

## Design principles
1. **Change capture** in the DB via the immutable `change_events` log (append-only).
2. **Sheet change detection**: use `spreadsheets.values.get` and a per-row checksum (`sha256(row_values)`) since the Sheets API has no native value-edit webhook. For large sheets use incremental windowing (a `modified_at` column in sheet).
3. **Mapping**: each relevant row must have `__row_id` (internal row id), `__last_synced_at`, and optional `__row_checksum` columns.
4. **Idempotency**: every sync operation carries `client_op_id` and `version`.
5. **Conflict resolution**: default policy: `max(version)` wins; tie-breaker: latest `last_modified`. Allow user preference of "sheet wins" or "app wins".

## High-level sync flow
1. Worker polls `sync_queue` for pending ops for user U.
2. For each `sheets_connection` of U:
   - Refresh token if needed.
   - Pull remote changes since `last_synced_at`:
     - Option A: if the sheet stores `__last_modified`, only fetch rows with newer timestamps.
     - Option B: compute row checksums and compare with stored shadow rows.
   - Pull local `change_events` since `last_synced_at`.
   - Compute a **merge plan**:
     - For each entity (by external id or mapped key):
       - if only local changed → push to sheet
       - if only remote changed → apply to DB (create/update)
       - if both changed → resolve by version or policy
   - Apply changes using batched `spreadsheets.values.batchUpdate` and DB transactional upserts.
   - Record `change_events` for resulting DB mutations.
   - Update `sheets_connections.last_synced_at` and mark queue items complete.

## Pseudocode (simplified)

```
function two_way_sync(connection):
  token = ensure_valid_token(connection)
  last_sync = connection.last_synced_at or epoch
  remote_rows = fetch_sheet_rows(connection.sheet_id, connection.sheet_range)
  local_changes = query_change_events(connection.user_id, since=last_sync)

  remote_map = map_remote_rows_by_row_id(remote_rows)
  local_map = map_local_changes_by_external_id(local_changes)

  ops = []

  for id in union(remote_map.keys(), local_map.keys()):
    remote = remote_map.get(id)
    local = local_map.get(id)

    if remote and not local:
      ops.append(apply_remote_to_db(remote))
    else if local and not remote:
      ops.append(push_local_to_sheet(local))
    else:
      // conflict
      if local.version > remote.version:
         ops.append(push_local_to_sheet(local))
      else if remote.version > local.version:
         ops.append(apply_remote_to_db(remote))
      else:
         // tie: use last_modified
         if local.last_modified >= remote.last_modified:
            ops.append(push_local_to_sheet(local))
         else:
            ops.append(apply_remote_to_db(remote))

  execute_ops(ops)
  update_last_synced_at(connection, now())
```

## Conflict resolution rules (recommended default)

1. Each row/entity must carry `version` (increment on each mutation).
2. Rule: **highest `version` wins**.
3. If versions equal, compare `last_modified` timestamp.
4. If tie persists, use user preference: `app_wins` or `sheet_wins`.
5. For deletions, prefer tombstones: set `deleted=true` and propagate.

## Failure handling & retries

- Use `sync_queue` with exponential backoff: `available_at = now() + base * 2^attempts`.
- Mark permanent failures after `max_attempts` and notify user with recovery instructions.
- Always write to `change_events` so operations can be replayed.

## Scalability patterns

- Shard workers by user hash or per Google project to avoid token rate limits.
- Throttle batch sizes to respect Sheets API quotas.
- Use incremental sync windows (time-range) rather than scanning full sheets for large spreadsheets.

---

# Folder structure — Go (backend)

```
/backend
├── cmd/
│   └── api/                     # main binary
├── internal/
│   ├── api/                     # http handlers
│   ├── auth/                    # oauth, token management
│   ├── db/                      # db models, migrations
│   ├── sync/                    # sync worker logic
│   ├── queue/                   # queue adapters (redis)
│   └── service/                 # business logic
├── pkg/
│   └── shared/                  # reusable libs (errors, utils)
├── migrations/                  # SQL migrations (golang-migrate)
├── docker/
│   └── Dockerfile
├── k8s/                         # optional k8s manifests
├── configs/
├── scripts/
└── Makefile
```

**Files to generate**
- `cmd/api/main.go`
- `internal/api/routes.go`
- `internal/sync/worker.go`
- `internal/db/models.go`
- `migrations/0001_init.sql`

---

# Folder structure — Flutter (mobile)

```
/mobile
├── lib/
│   ├── main.dart
│   ├── app.dart
│   ├── src/
│   │   ├── models/
│   │   ├── services/
│   │   │   ├── api_service.dart
│   │   │   ├── sync_service.dart
│   │   │   └── sheets_integration.dart
│   │   ├── db/
│   │   │   └── local_db.dart    # sqflite / sembast schema
│   │   ├── ui/
│   │   └── state/               # riverpod / provider
│   └── utils/
├── android/
├── ios/
└── pubspec.yaml
```

**Key modules**
- `sync_service.dart`: queue local ops, retry, conflict resolution hooks.
- `local_db.dart`: local mirror of Postgres schema for offline ops; stores `pending_ops` table.
- `sheets_integration.dart`: handles Google Sign-In and stores tokens securely.

---

# Folder structure — Next.js (web)

```
/web
├── app/ or pages/                # Next.js 13 app router or pages router
│   ├── dashboard/
│   ├── transactions/
│   └── sheets/
├── components/
├── lib/
│   ├── api/                      # API client wrappers
│   ├── auth/                     # next-auth helpers
│   └── hooks/
├── public/
├── styles/
├── next.config.js
└── package.json
```

**Notes**
- Use CSR for interactive UI; use SSR for specific dashboard pages only when needed.
- Consider NextAuth.js or custom JWT middleware for sessions.

---

# Prompt-ready snippets — generate the whole project

Below are prompt blocks you can paste into a code assistant (Cursor/GPT) to generate skeleton files. Each block indicates filename, language, and a short requirement.

### PROMPT: generate file `backend/cmd/api/main.go`

```text
### PROMPT: generate file backend/cmd/api/main.go
Language: go
Requirement:
Create `main.go` for the `api` binary that:
- uses chi router
- loads config from env (DB_URL, JWT_SECRET, GOOGLE_CLIENT_ID/SECRET)
- initializes DB (sqlx), Redis client, and the sync worker pool
- registers basic routes: /health, /auth/oauth-callback, /api/v1/transactions
- is Docker-friendly (logs to stdout)
Provide full code with imports and short comments.
```

---

### PROMPT: generate file `backend/internal/sync/worker.go`

```text
### PROMPT: generate file backend/internal/sync/worker.go
Language: go
Requirement:
Create `worker.go` that:
- consumes rows from Postgres `sync_queue`
- refreshes Google tokens using stored refresh_token
- executes two-way sync core loop (pseudocode acceptable)
- emits logs and metrics
Include functions: StartWorker(ctx) and processQueueItem(item).
```

---

### PROMPT: generate file `migrations/0001_init.sql`

```text
### PROMPT: generate file migrations/0001_init.sql
Language: sql
Requirement:
Create the initial migration SQL that defines tables: users, accounts, categories, transactions, change_events, sheets_connections, sheet_mappings, sync_queue.
Use Postgres syntax with constraints and indexes.
```

---

### PROMPT: generate file `mobile/lib/src/db/local_db.dart`

```text
### PROMPT: generate file mobile/lib/src/db/local_db.dart
Language: dart
Requirement:
Create Dart `local_db.dart` using sqflite:
- define schema for transactions, pending_ops, devices
- provide singleton LocalDatabase class with init(), upsertTransaction(), enqueueOp()
- example method to read pending ops
Include imports and a usage example.
```

---

### PROMPT: generate file `web/lib/api/transactions.ts`

```text
### PROMPT: generate file web/lib/api/transactions.ts
Language: typescript
Requirement:
Create a TypeScript API helper for Next.js:
- functions fetchTransactions(token), createTransaction(payload), syncNow()
- use fetch API and handle 401 with a refresh fallback
- include types for Transaction and ApiError
```

---

### PROMPT: generate file `.github/workflows/ci.yml`

```text
### PROMPT: generate file .github/workflows/ci.yml
Language: yaml
Requirement:
Create a GitHub Actions CI pipeline that:
- checks out code, sets up Go and Node
- runs lints and tests for backend, web, and mobile
- builds a Docker image for backend and pushes it on main branch (use secrets.REGISTRY)
```

---

# Additional engineering notes & best practices

- **Idempotency:** every queue operation must carry an `idempotency_key`.
- **Transactions:** use DB transactions for multi-row writes and record `change_events` within the same transaction.
- **Token storage:** encrypt refresh tokens at rest (KMS or environment key).
- **Rate limits:** implement per-user throttling for Sheets calls and add jitter to retries.
- **Observability:** instrument critical paths (sync duration, rows processed, failures) with OpenTelemetry.
- **Backfills & replays:** implement `replay_change_events(user_id, since)` to rebuild state after fixes.
- **Testing:** create integration tests that mock the Sheets API using recorded fixtures.
- **Security:** do not store long-lived access_tokens in plaintext; rotate keys and credentials regularly.

---

# Quick-start checklist (to reach ~70–80% auto-generated code)

1. Generate and apply migrations (`migrations/0001_init.sql`) to your chosen Postgres (Supabase/Neon).
2. Generate `cmd/api/main.go`, basic handlers, and Dockerfile.
3. Generate `internal/sync/worker.go` and a consumer for `sync_queue`.
4. Generate Flutter `local_db.dart`, basic models, and `sync