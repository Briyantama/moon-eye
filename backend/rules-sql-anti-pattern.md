## SQL Anti-Pattern Rules (Improved)

This document refines `sql-anti-pattern.txt` into a set of practical, PostgreSQL‑focused rules for **query design** and **schema design** in this backend.

Rules are grouped by concern and tagged with severity:

- **[C] Critical** — can cause correctness or security issues.
- **[H] High** — performance / maintainability problems that will hurt in production.
- **[M] Medium** — non-ideal but acceptable short term.
- **[L] Low** — style / clarity improvements.

---

## 1. Query shape & result set

1. **Avoid `SELECT *` in application queries** **[H]**

   - Use an explicit column list for all `SELECT` and `RETURNING` clauses used by application code.
   - `SELECT *` is only allowed in:
     - Ad‑hoc debugging.
     - One‑off admin scripts outside the application codebase.

   **Why**:
   - Coupling to table schema; adding columns can break scans silently.
   - Over-fetching data, harder to review for sensitive columns.

2. **Always have a `WHERE` clause for data-modifying operations** **[C]**

   - `UPDATE` and `DELETE` without a `WHERE` clause are forbidden.
   - If a bulk update is truly required, it must be:
     - Reviewed explicitly in a migration script.
     - Documented with comments explaining why it is safe.

3. **Avoid unbounded `SELECT` on large tables** **[H]**

   - For tables that grow unbounded (`transactions`, `change_events`, `sync_queue`):
     - `SELECT` must use at least one of:
       - `WHERE` that filters by tenant/user/time window.
       - `LIMIT` with a sensible cap (plus pagination).
   - Exceptions:
     - Small reference tables (e.g. currencies) with documented size constraints.

4. **Pagination is mandatory for list endpoints** **[H]**

   - Any `SELECT` used by a list API must:
     - Include `LIMIT` and `OFFSET` (or `keyset pagination` in future).
     - Have a deterministic `ORDER BY`.

5. **Use `COUNT(*)` only for counting** **[M]**

   - `COUNT(*)` is acceptable and efficient in Postgres.
   - Do not use `SELECT COUNT(*)` as a proxy for checking existence when `EXISTS` is clearer:

   ```sql
   SELECT EXISTS (
     SELECT 1 FROM users WHERE id = $1
   );
   ```

---

## 2. Parameters, predicates & functions

6. **Never build SQL by concatenating untrusted strings** **[C]**

   - All SQL must use positional parameters (`$1`, `$2`, …) or sqlc‑generated parameter structs.
   - Dynamic SQL composed with `fmt.Sprintf` / string concatenation is allowed **only** when:
     - The interpolated fragments are not user‑controlled (e.g. optional filters by column that are NOT names).
     - Even then, the preferred approach is to model optional filters as nullable parameters in sqlc queries.

7. **Avoid applying functions to indexed columns in WHERE/JOIN** **[H]**

   - Predicates like `LOWER(email) = LOWER($1)` prevent index-only plans unless there is a functional index.
   - Preferred pattern:

   ```sql
   -- Create index on LOWER(email)
   CREATE INDEX idx_users_email_lower ON users (LOWER(email));

   -- Then:
   WHERE LOWER(email) = LOWER($1)
   ```

   - If no functional index exists, avoid wrapping the column in a function in WHERE/JOIN predicates.

8. **Avoid `LIKE` with leading wildcard for large tables** **[H]**

   - `WHERE column LIKE '%foo'` disables index usage in normal btree indexes.
   - Use:
     - Trigram/GiST/GiN indexes if needed (`pg_trgm`).
     - Or restrict to admin/debug queries, not hot paths.

9. **Do not use query hints / optimizer hints** **[M]**

   - Postgres does not support traditional hints; any vendor-specific hint syntax must be avoided.

---

## 3. Concurrency & locking

10. **Use optimistic locking for frequently updated rows** **[H]**

   - For entities like `transactions`:
     - Table must have a `version BIGINT NOT NULL`.
     - `UPDATE` and `soft delete` queries must **both**:
       - Increment `version`: `version = version + 1`.
       - Check the old version in WHERE: `AND version = $expected`.
   - Application must:
     - Pass expected version from the domain model.
     - Map `ErrNoRows` on update/delete to a **version conflict** (e.g. HTTP 409).

11. **Avoid ad‑hoc `SELECT … FOR UPDATE` in application code** **[M]**

   - If you need pessimistic locking, encapsulate it in a repository method with clear semantics.

---

## 4. Schema & keys

12. **Prefer single-column primary keys** **[H]**

   - Composite primary keys are allowed only for:
     - Proven performance reasons in projections or junction tables.
   - Else:
     - Use a surrogate key (UUID or BIGSERIAL) plus separate unique constraints as needed.

13. **Primary key type** **[M]**

   - Prefer `UUID` (with `gen_random_uuid()` in Postgres) or `BIGSERIAL`.
   - `UNIQUEIDENTIFIER` from original rules is SQL Server‑specific; in Postgres, UUID is equivalent.

14. **Foreign keys should be NOT NULL by default** **[H]**

   - Nullable FKs are allowed only when “no parent” is a meaningful business state.
   - Otherwise, define as `NOT NULL` and use proper referential constraints.

15. **Columns used in equality predicates should avoid NULL where possible** **[M]**

   - For search keys (e.g. user ids, external ids):
     - Prefer `NOT NULL`.
     - If null is required, design the API to account for tri‑state logic.

---

## 5. Stored procedures, triggers & views

16. **Avoid triggers unless strictly necessary** **[H]**

   - Triggers hide write paths and make behavior non-obvious.
   - Allowed only when:
     - Business‑critical.
     - Reviewed and documented (what they do, when they fire).

17. **Avoid user-defined functions (UDFs) in hot paths** **[M]**

   - Table‑valued functions / scalar UDFs in SELECT lists or WHERE/JOIN predicates can cause:
     - RBAR (“row by agonizing row”) execution.
     - Hard‑to‑predict performance.

---

## 6. Indexes & performance

18. **Always index foreign keys and common filters** **[H]**

   - For each FK column used frequently in joins or filters, create an index.
   - For big tables (`transactions`, `change_events`, `sync_queue`), ensure:
     - Composite indexes match query patterns (e.g. `(user_id, last_modified DESC)`).

19. **Avoid table scans on hot paths** **[H]**

   - Profile queries; any that frequently scan entire large tables should be:
     - Rewritten to use indexed predicates.
     - Or offloaded to projections / reporting tables.

---

## 7. Application-side patterns

20. **Centralize SQL in `internal/db/queries` with sqlc** **[H]**

   - All persistent queries should live in `.sql` files consumed by sqlc.
   - Inline static SQL in Go is allowed only when:
     - It is trivial, low‑level, and not reused across services.
     - Or during a migration phase before being moved into sqlc.

21. **Avoid dynamic SQL in application code** **[H]**

   - Building queries via string concatenation is considered dynamic SQL.
   - Prefer expressing optional filters as nullable parameters in a single sqlc query.

22. **No CURSOR / WHILE loops in application queries** **[H]**

   - Cursors and WHILE+SELECT loops lead to RBAR behavior.
   - Use set-based operations; for background workers, batch by window (`LIMIT`) and process in application code.

---

## 8. Result set size & streaming

23. **Limit maximum row counts** **[H]**

   - For API list endpoints, the maximum `LIMIT` should be reasonable (e.g. 200).
   - Enforce upper bounds in service code before passing to queries.

24. **Consider streaming only when needed** **[M]**

   - For very large exports (e.g. full transaction history), design dedicated streaming endpoints instead of lifting limits for general list APIs.

---

## 9. Documentation & review

25. **Document exceptions to rules** **[M]**

   - Any query that intentionally violates one of these rules (e.g. global admin list on `change_events` without `WHERE`) must:
     - Be clearly commented in the SQL.
     - Be referenced in architecture docs or ADRs.

26. **All new queries must be reviewed for anti-patterns** **[H]**

   - Code review checklist should include:
     - No `SELECT *`.
     - `UPDATE`/`DELETE` have `WHERE`.
     - Pagination and `ORDER BY` for lists.
     - No dynamic SQL in Go; use sqlc.
     - Indexes exist (or are planned) for key filters.

