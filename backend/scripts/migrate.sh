#!/usr/bin/env sh

set -euo pipefail

DB_URL="${DB_URL:-postgres://postgres:postgres@localhost:5432/finance?sslmode=disable}"

echo "Running migrations against ${DB_URL}"

# TODO: ensure golang-migrate CLI is installed in the environment.
migrate -database "${DB_URL}" -path "./migrations" up

