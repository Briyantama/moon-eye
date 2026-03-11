# Moon Eye Backend

Go backend for the Moon Eye finance app: REST APIs, auth, transactions, and Google Sheets sync. Built as **independent microservices** that share a common codebase and communicate via HTTP and Redis Streams.

**→ [Architecture diagram and modules](docs/ARCHITECTURE.md)** — system context, services, data flow, and codebase structure.

---

## Architecture

| Service | Port (default) | Description |
|--------|----------------|--------------|
| **finance-api** | `:8080` | Main API: transactions CRUD, auth (register/login/refresh), sheets connections, sync trigger. Runs migrations; health + metrics. |
| **auth-service** | `:8081` | Dedicated auth service (JWT issuance, OAuth2) — stub for now; auth is currently in finance-api. |
| **sync-service** | `:8082` | Consumes `sync_queue` from Redis; runs Google Sheets sync jobs (connections, mappings, change_events). |
| **worker-service** | — | Projection worker: reads `change_events`, updates `transaction_summary` and `monthly_balance`. No HTTP server. |

Shared components:

- **PostgreSQL** — primary store (users, accounts, categories, transactions, change_events, sheets_connections, sheet_mappings).
- **Redis** — stream `sync_queue` for async sync jobs; consumer group `sync_workers`.
- **Config** — `configs/{APP_ENV}.yaml` + env overrides per service (e.g. `FINANCE_API_DATABASE_URL`).

---

## Prerequisites

- **Go 1.22+**
- **PostgreSQL 15+**
- **Redis 6+** (optional for finance-api if you skip sync; required for sync-service)
- **Make** (optional; for `make` targets below)

---

## How to Start

### 1. Database and Redis

Create a database and run migrations:

```bash
# Create DB (example for local Postgres)
createdb moon_eye

# Run migrations (from backend directory)
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/moon_eye?sslmode=disable"
make migrate
```

Use a `configs/dev.yaml` (see [Configuration](#configuration)) or set env vars so that `database.url` points to your Postgres and (if needed) `redis.url` to Redis.

### 2. Configuration

Config is loaded from `configs/{APP_ENV}.yaml` (default `APP_ENV=dev` → `configs/dev.yaml`).  
Each service uses its own **env prefix** for overrides (see `configs/dev.yaml`):

- finance-api: `FINANCE_API_*` (e.g. `FINANCE_API_DATABASE_URL`, `FINANCE_API_SERVER_ADDR`)
- auth-service: `AUTH-SERVICE_*`
- sync-service: `SYNC-SERVICE_*`
- worker-service: `WORKER-SERVICE_*`

Example `configs/dev.yaml` (create from this if missing):

```yaml
server:
  addr: ":8080"
database:
  url: "postgres://postgres:postgres@localhost:5432/moon_eye?sslmode=disable"
redis:
  url: "redis://localhost:6379/0"
auth:
  jwtSecret: "your-secret"
logging:
  level: "debug"
```

Override per service, e.g.:

```bash
export FINANCE_API_DATABASE_URL="postgres://..."
export FINANCE_API_SERVER_ADDR=":8080"
export FINANCE_API_REDIS_URL="redis://localhost:6379/0"
export JWT_SIGNING_KEY="your-secret"   # or set auth.jwtSecret in config
```

### 3. Run with Make (recommended)

From the **backend** directory:

```bash
# Run one service
make run-finance-api    # :8080
make run-auth-service   # :8081
make run-sync-service   # :8082
make run-worker-service # projections only

# Run all services (each in background)
make run-all
```

These targets set `APP_ENV=dev`, use `configs/dev.yaml`, and override `SERVER_ADDR` per service so ports don’t clash.

### 4. Run without Make

From the **backend** directory, with config and env set:

```bash
export APP_ENV=dev

# Finance API (transactions, auth, health, metrics)
go run ./cmd/api/finance-api

# Auth service (stub)
go run ./cmd/api/auth-service

# Sync service (Redis consumer + HTTP health)
go run ./cmd/api/sync-service

# Worker service (projections only)
go run ./cmd/api/worker-service
```

Set `*_DATABASE_URL`, `*_REDIS_URL`, and `*_SERVER_ADDR` per service if you’re not using `configs/dev.yaml`.

### 5. Docker

Build all binaries and run one service via `SERVICE_NAME`:

```bash
docker build -t moon-eye-backend .
docker run --rm -e SERVICE_NAME=finance-api \
  -e FINANCE_API_DATABASE_URL="postgres://..." \
  -e FINANCE_API_REDIS_URL="redis://..." \
  -p 8080:8080 moon-eye-backend
```

For a full local stack (Postgres, Redis, services), use your existing `docker-compose` setup.

---

## Makefile Targets

| Target | Description |
|--------|--------------|
| `make run-finance-api` | Start finance-api on `:8080` |
| `make run-auth-service` | Start auth-service on `:8081` |
| `make run-sync-service` | Start sync-service on `:8082` |
| `make run-worker-service` | Start worker-service (no HTTP) |
| `make run-all` | Start all four in the background |
| `make build` | Build finance-api binary |
| `make build-all` | Build all service binaries |
| `make test` | Run unit tests |
| `make test-integration` | Run integration tests (Docker) |
| `make migrate` | Run DB migrations (requires `DATABASE_URL`) |
| `make docker-build` | Build Docker image |
| `make clean` | Remove built binaries |

---

## Testing

- **Unit tests** (no Docker):

  ```bash
  go test ./...
  # or
  make test
  ```

- **Integration tests** (PostgreSQL via testcontainers; requires Docker):

  ```bash
  INTEGRATION_DB=1 go test -tags=integration ./tests/integration -v
  # or
  make test-integration
  ```

Run from the `backend` directory. Migrations are applied automatically against an ephemeral Postgres 15 container.

---

## Sync Service and Redis Streams

### sync_queue contract

The sync service and finance-api communicate via a Redis Stream named `sync_queue`. Entries use:

- **entity**: logical entity type, e.g. `transaction` or `account`.
- **operation**: e.g. `create`, `update`, `delete`, or `full_sync`.
- **payload**: JSON-encoded job payload.
- **retry_count**: integer (optional, default 0).

Example job payload for transaction syncs:

```json
{
  "userId": "uuid-of-user",
  "connectionId": "uuid-of-sheets-connection",
  "mode": "two-way",
  "sinceVersion": 42
}
```

Consumed by `SyncService` via `SyncJobPayload`; produced by `TransactionService` (and others) through the `SyncQueue` abstraction.

The sync worker uses consumer group `sync_workers` on stream `sync_queue`. Each sync-service instance uses a distinct consumer name (e.g. `sync-service`) for horizontal scaling.

### Sync service behavior

- **Config**: `pkg/shared/config` (DB URL, Redis URL, server address).
- **DB**: `pgxpool.Pool` via `pkg/shared/db`.
- **Redis**: `pkg/shared/redisx`; `internal/queue.RedisStreamQueue` for `sync_queue` + group `sync_workers`.
- **Worker**: `internal/worker.Runner` consumes jobs and calls sync domain logic.

Domain sync lives under `internal/sync`:

- `SheetsClient` (e.g. `NoopSheetsClient` for dev).
- `SyncService`: loads connections/mappings, reads `change_events`, converts to `SheetRowChange`, fetches/applies changes via `SheetsClient`.

### Dataflow

```mermaid
flowchart LR
  financeAPI[finance-api] --> syncQueue[Redis sync_queue]
  syncQueue --> syncServiceWorker[sync-service worker]
  syncServiceWorker --> syncService[SyncService]
  syncService --> postgres[Postgres]
  syncService --> sheetsClient[GoogleSheetsClient]
  sheetsClient --> googleSheets[Google Sheets]
```

- finance-api (and other writers) enqueue jobs to `sync_queue`.
- sync-service consumes via the consumer group and runs `SyncService`.
- SyncService reads from Postgres and (when configured) Google Sheets and writes back via repositories and `SheetsClient`.
