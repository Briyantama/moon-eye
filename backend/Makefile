# Moon Eye Backend — run and build all services from backend directory.
# Requires: Go 1.22+, Postgres, Redis (for sync). Set DB/Redis in configs/dev.yaml or env.

APP_ENV ?= dev
GO ?= go
BIN_DIR ?= bin

# Default DB/Redis for migrate and local runs (override in configs/dev.yaml or env).
DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/moon_eye?sslmode=disable
REDIS_URL ?= redis://localhost:6379/0

.PHONY: run-finance-api run-auth-service run-sync-service run-worker-service run-all
.PHONY: build build-all test test-integration migrate docker-build clean

# ---- Run services (APP_ENV=dev, configs/dev.yaml) ----
run-finance-api:
	APP_ENV=$(APP_ENV) FINANCE_API_SERVER_ADDR=:8080 FINANCE_API_DATABASE_URL="$(DATABASE_URL)" FINANCE_API_REDIS_URL="$(REDIS_URL)" \
	$(GO) run ./cmd/api/finance-api

run-auth-service:
	APP_ENV=$(APP_ENV) AUTH-SERVICE_SERVER_ADDR=:8081 AUTH-SERVICE_DATABASE_URL="$(DATABASE_URL)" \
	$(GO) run ./cmd/api/auth-service

run-sync-service:
	APP_ENV=$(APP_ENV) SYNC-SERVICE_SERVER_ADDR=:8082 SYNC-SERVICE_DATABASE_URL="$(DATABASE_URL)" SYNC-SERVICE_REDIS_URL="$(REDIS_URL)" \
	$(GO) run ./cmd/api/sync-service

run-worker-service:
	APP_ENV=$(APP_ENV) WORKER-SERVICE_DATABASE_URL="$(DATABASE_URL)" \
	$(GO) run ./cmd/api/worker-service

# Run all services in background. Ctrl+C kills make; background processes may need pkill.
run-all:
	@echo "Starting finance-api :8080, auth-service :8081, sync-service :8082, worker-service..."
	@(APP_ENV=$(APP_ENV) FINANCE_API_SERVER_ADDR=:8080 FINANCE_API_DATABASE_URL="$(DATABASE_URL)" FINANCE_API_REDIS_URL="$(REDIS_URL)" $(GO) run ./cmd/api/finance-api &); \
	(APP_ENV=$(APP_ENV) AUTH-SERVICE_SERVER_ADDR=:8081 AUTH-SERVICE_DATABASE_URL="$(DATABASE_URL)" $(GO) run ./cmd/api/auth-service &); \
	(APP_ENV=$(APP_ENV) SYNC-SERVICE_SERVER_ADDR=:8082 SYNC-SERVICE_DATABASE_URL="$(DATABASE_URL)" SYNC-SERVICE_REDIS_URL="$(REDIS_URL)" $(GO) run ./cmd/api/sync-service &); \
	(APP_ENV=$(APP_ENV) WORKER-SERVICE_DATABASE_URL="$(DATABASE_URL)" $(GO) run ./cmd/api/worker-service &); \
	wait

# ---- Build ----
build:
	$(GO) build -o $(BIN_DIR)/finance-api ./cmd/api/finance-api

build-all:
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/finance-api ./cmd/api/finance-api
	$(GO) build -o $(BIN_DIR)/auth-service ./cmd/api/auth-service
	$(GO) build -o $(BIN_DIR)/sync-service ./cmd/api/sync-service
	$(GO) build -o $(BIN_DIR)/worker-service ./cmd/api/worker-service
	@echo "Binaries in $(BIN_DIR)/"

# ---- Test ----
test:
	$(GO) test ./...

test-integration:
	INTEGRATION_DB=1 $(GO) test -tags=integration ./tests/integration -v

# ---- Migrate (requires DATABASE_URL) ----
migrate:
	@which migrate >/dev/null || (echo "install golang-migrate: https://github.com/golang-migrate/migrate"; exit 1)
	migrate -path migrations -database "$(DATABASE_URL)" up

# ---- Docker ----
docker-build:
	docker build -t moon-eye-backend .

# ---- Clean ----
clean:
	rm -rf $(BIN_DIR)
