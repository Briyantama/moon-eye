## SQL Anti-Pattern Report

This report is based on `sql-anti-pattern.txt` plus additional best-practice rules. It covers `.sql` files and inline SQL in Go under `backend/internal`.

---

### 1. Findings in `.sql` files (`internal/db/queries`)

#### 1.1 `internal/db/queries/transactions.sql`

```1:203:internal/db/queries/transactions.sql
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
  deleted
FROM transactions
WHERE user_id = $1
  AND deleted = false
ORDER BY occurred_at DESC
LIMIT $2 OFFSET $3;
```

- **Assessment**:
  - Uses explicit column list (no `SELECT *`) ✅.
  - Has `WHERE` and pagination (`LIMIT/OFFSET`) ✅.
  - Uses indexed columns (`user_id`, `deleted`, `occurred_at`) consistent with schema indexes ✅.
- **Rule mapping**:
  - Avoid wildcard `SELECT *` (rule 19) — compliant.
  - `SELECT` without `WHERE` (rule 20) — not violated.
- **Priority / remediation**: **None** – query is already optimal and aligned with rules.

```24:61:internal/db/queries/transactions.sql
-- name: CreateTransaction :one
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
  deleted
) VALUES (
  $1, $2, $3, $4, $5,
  $6, $7, $8, $9,
  $10, $11, $12, $13, $14
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
  deleted;
```

- **Assessment**:
  - Explicit insert column list + `RETURNING` list ✅.
  - No anti-patterns per `sql-anti-pattern.txt`.
- **Priority**: **None**.

```63:69:internal/db/queries/transactions.sql
-- name: CountTransactionsByUser :one
SELECT COUNT(*)::bigint AS total
FROM transactions
WHERE user_id = $1
  AND deleted = false;
```

- **Assessment**:
  - `COUNT(*)` is acceptable here (aggregation) – rule 19 about wildcard results does **not** apply.
  - Proper `WHERE` clause, index-friendly ✅.
- **Priority**: **None**.

```70:92:internal/db/queries/transactions.sql
-- name: GetTransactionByID :one
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
  deleted
FROM transactions
WHERE id = $1
  AND user_id = $2
  AND deleted = false;
```

- **Assessment**:
  - Explicit column list, filtered by PK and user ✅.
  - No anti-patterns detected.
- **Priority**: **None**.

```93:112:internal/db/queries/transactions.sql
-- name: GetTransactionByIDOnly :one
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
  deleted
FROM transactions
WHERE id = $1;
```

- **Assessment**:
  - Explicit column list ✅.
  - No `deleted = false` filter by design (used internally in transactional flows). Not an anti-pattern, but should **not** be used directly by HTTP layer.
- **Priority**: **Low**.
- **Remediation**:
  - Ensure this query is only used in internal flows (repository/service), not directly exposed to API handlers.

```114:148:internal/db/queries/transactions.sql
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
  version       = version + 1
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
  deleted;
```

- **Assessment**:
  - Optimistic locking via `AND version = $13` ✅ (aligns with rules; not in `sql-anti-pattern.txt` but is best practice).
  - Explicit column list and `WHERE` with PK + user filter ✅.
  - No anti-patterns.
- **Priority**: **None**.

```150:201:internal/db/queries/transactions.sql
-- name: SoftDeleteTransaction :one
-- Soft-delete a transaction (id + user_id).
UPDATE transactions
SET
  deleted       = true,
  last_modified = now(),
  version       = version + 1
WHERE id = $1
  AND user_id = $2
  AND deleted = false
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
  deleted;
```

- **Assessment**:
  - Update has `WHERE` by PK+user+deleted flag ✅ (not rule‑26 anti‑pattern).
  - Version bump but **no optimistic check in WHERE** (so last-writer-wins is possible).
- **Priority**: **Medium**.
- **Remediation**:
  - Introduce an expected version param in `SoftDelete` query if you want full optimistic locking:
    - Add `AND version = $3` and thread `Version` from caller.
    - Map `ErrNoRows` to conflict in service.  
  - Currently acceptable but slightly weaker than `UpdateTransaction`.

```177:201:internal/db/queries/transactions.sql
-- name: SoftDeleteTransactionByID :one
-- Soft-delete a transaction by id only (for use when user_id not in scope).
UPDATE transactions
SET
  deleted       = true,
  last_modified = now(),
  version       = version + 1
WHERE id = $1
  AND deleted = false
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
  deleted;
```

- **Assessment**:
  - `UPDATE` has `WHERE` (by id + deleted flag) ✅.
  - No version guard; susceptible to last-writer-wins like above.
- **Priority**: **Medium**.
- **Remediation**:
  - Same as above: optional improvement to add version-based optimistic locking.

#### 1.2 `internal/db/queries/users.sql`

```1:18:internal/db/queries/users.sql
-- name: CreateUser :exec
INSERT INTO users (id, email, hashed_password, created_at, updated_at, deleted)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetUserByEmail :one
SELECT id, email, hashed_password, created_at, updated_at, deleted
FROM users
WHERE LOWER(email) = LOWER($1) AND deleted = false;

-- name: GetUserByID :one
SELECT id, email, hashed_password, created_at, updated_at, deleted
FROM users
WHERE id = $1 AND deleted = false;

-- name: UpdateUser :exec
UPDATE users
SET email = $2, hashed_password = $3, updated_at = $4, deleted = $5
WHERE id = $1 AND deleted = false;
```

- **Assessment**:
  - No `SELECT *` (explicit columns) ✅.
  - All `SELECT` have `WHERE` ✅.
  - `UpdateUser` has `WHERE id = …` (no rule‑26 issue).
  - Optimistic locking is **not** enforced here (no `version` in `WHERE`), but that is an enhancement; not an anti-pattern.
- **Priority**: **Low**.
- **Remediation** (optional):
  - Add optimistic locking for `users` similar to `transactions` when the domain requires strict concurrency control.

#### 1.3 `internal/db/queries/refresh_tokens.sql`

```1:18:internal/db/queries/refresh_tokens.sql
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
```

- **Assessment**:
  - No `SELECT *`; explicit columns ✅.
  - All selects have `WHERE` ✅.
  - `UPDATE refresh_tokens SET revoked = true WHERE id = $1` has proper `WHERE` ✅.
- **Priority**: **None** (no rule from `sql-anti-pattern.txt` violated).

#### 1.4 `internal/db/queries/change_events.sql`

```1:40:internal/db/queries/change_events.sql
-- name: InsertChangeEvent :exec
INSERT INTO change_events (
  entity_type,
  entity_id,
  user_id,
  op_type,
  version,
  payload,
  created_at
) VALUES (
  $1, $2, $3, $4, $5, $6, NOW()
);

-- name: ListChangeEvents :many
SELECT
  id,
  entity_type,
  entity_id,
  user_id,
  op_type,
  version,
  payload,
  created_at
FROM change_events
ORDER BY created_at DESC, id DESC
LIMIT $1 OFFSET $2;

-- name: GetChangeEventByID :one
SELECT
  id,
  entity_type,
  entity_id,
  user_id,
  op_type,
  version,
  payload,
  created_at
FROM change_events
WHERE id = $1;
```

- **Assessment**:
  - No `SELECT *` ✅.
  - `ListChangeEvents` has **no WHERE** but is paginated and ordered.
  - Per rule‑20 (“SELECT without WHERE”), this is a soft anti-pattern when table grows large.
- **Priority**: **Medium**.
- **Remediation**:
  - For end-user or tenant-scoped views, add variant filtered by `user_id` and/or `entity_type`:

    ```sql
    -- name: ListUserChangeEvents :many
    SELECT id, entity_type, entity_id, user_id, op_type, version, payload, created_at
    FROM change_events
    WHERE user_id = $1
    ORDER BY created_at DESC, id DESC
    LIMIT $2 OFFSET $3;
    ```

  - Keep the global `ListChangeEvents` for admin/debug tooling only.

---

### 2. Inline SQL in Go (`internal/db/*.go`)

#### 2.1 `internal/db/transaction_repository_impl.go`

```145:176:internal/db/transaction_repository_impl.go
base := `
SELECT id, user_id, account_id, amount, currency, type, category_id, description,
       occurred_at, metadata, version, last_modified, source, sheets_row_id, deleted
FROM transactions
WHERE user_id = $1 AND deleted = false
`
args := []any{filter.UserID}
pos := 2
if filter.AccountID != nil {
    base += fmt.Sprintf(" AND account_id = $%d", pos)
    ...
}
...
base += " ORDER BY occurred_at DESC"
base += fmt.Sprintf(" LIMIT $%d OFFSET $%d", pos, pos+1)
args = append(args, filter.Limit, filter.Offset)
```

- **Assessment**:
  - This is dynamic SQL composed via string concatenation in Go, driven only by filter presence (no user-controlled fragments) – safe from injection but:
    - **Violates rule:** “Query with usage of Dynamic SQL on application side” (rule 39).
    - Makes it harder to reason about plan caching and index usage.
  - It also duplicates logic that could be represented as extra WHERE clauses in a sqlc query.
- **Priority**: **High (from maintainability perspective)**.
- **Remediation**:
  - Move this query into `internal/db/queries/transactions.sql` with optional filters expressed as parameters:

    ```sql
    -- name: ListTransactionsByFilter :many
    SELECT id, user_id, account_id, amount, currency, type, category_id, description,
           occurred_at, metadata, version, last_modified, source, sheets_row_id, deleted
    FROM transactions
    WHERE user_id = $1
      AND deleted = false
      AND ($2::uuid IS NULL OR account_id = $2)
      AND ($3::text IS NULL OR type = $3)
      AND ($4::timestamptz IS NULL OR occurred_at >= $4)
      AND ($5::timestamptz IS NULL OR occurred_at <= $5)
    ORDER BY occurred_at DESC
    LIMIT $6 OFFSET $7;
    ```

  - Generate sqlc and replace `listWithFilters` with a call to `ListTransactionsByFilter`, passing `NULL` for unused filters.

#### 2.2 `internal/db/sync_repositories.go`

Queries in this file use inline SQL but are **static**, not dynamically concatenated, and follow best practices:

- `GetByID` and `ListActiveByUser`:

```20:25:internal/db/sync_repositories.go
const q = `
    SELECT id, user_id, sheet_id, COALESCE(sheet_range, ''), sync_mode, EXTRACT(EPOCH FROM last_synced_at)
    FROM sheets_connections
    WHERE id = $1
`
```

- `ListByConnection`:

```101:107:internal/db/sync_repositories.go
const q = `
    SELECT id, connection_id, sheet_column, db_field, transform
    FROM sheet_mappings
    WHERE connection_id = $1
    ORDER BY sheet_column
`
```

- `ListTransactionEventsSince`:

```142:151:internal/db/sync_repositories.go
const q = `
    SELECT id, entity_type, entity_id, user_id, op_type, version, payload, created_at
    FROM change_events
    WHERE user_id = $1
      AND entity_type = 'transaction'
      AND version > $2
    ORDER BY version ASC
    LIMIT $3
`
```

- **Assessment**:
  - No dynamic string concatenation — **not** rule‑39.
  - All have `WHERE` + `LIMIT` where appropriate.
  - They could be migrated to sqlc for consistency but are **not** anti-patterns.
- **Priority**: **Low**.
- **Remediation** (optional):
  - Move these to `internal/db/queries/*.sql` and let sqlc generate typed accessors to remove manual scanning.

---

### 3. Findings in migrations

Migrations define schema; the `sql-anti-pattern.txt` rules mostly apply at query/DML level. Key checks:

- Primary keys are single-column UUID or BIGSERIAL (rules 6–8) ✅.
- No `sql_variant`, `xml`, `cursor`, `sp_`/`fn_` prefixes, or `SELECT INTO`/`TOP(1)` patterns ✅.
- Indexes exist on main foreign keys and change_events/sync_queue.

No critical anti-patterns found in migrations relative to the rules provided.

---

### 4. Summary of Anti-Patterns & Priorities

| Area | File | Lines | Issue | Priority | Suggested Fix |
|------|------|-------|-------|----------|---------------|
| Dynamic SQL | `internal/db/transaction_repository_impl.go` | 145–176 | Query constructed via string concatenation in app (rule 39: dynamic SQL on application side) | **High** | Move filtered list query into `internal/db/queries/transactions.sql` as a sqlc query with nullable filter params. |
| Global list | `internal/db/queries/change_events.sql` | 15–26 | `SELECT` from `change_events` without `WHERE` (rule 20; though paginated) | **Medium** | Add user-scoped variant; keep global list only for admin/ops. |
| Soft delete locking | `internal/db/queries/transactions.sql` | 150–201 | `SoftDelete*` bump `version` but don’t check it in `WHERE` (potential lost update) | **Medium** | Optionally add version-based `WHERE` and propagate expected version from callers. |
| Users optimistic lock | `internal/db/queries/users.sql` | 14–17 | `UpdateUser` doesn’t enforce version (design choice, not anti-pattern) | Low | If user record concurrency becomes an issue, add optimistic locking similar to transactions. |
| Inline static SQL | `internal/db/sync_repositories.go` | 20–25, 51–55, 101–107, 142–151 | Static inline SQL (no dynamic parts) | Low | Optionally migrate to sqlc to centralize queries and types. |

Overall, the most important remediation is to **remove dynamic SQL building from `transaction_repository_impl`** and replace it with sqlc-generated, parameterized queries. The rest of the queries already avoid `SELECT *` and unsafe `UPDATE/DELETE` patterns, with only small opportunities for stronger optimistic locking and user-scoped views.

