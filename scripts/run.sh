#!/usr/bin/env bash

set -e

echo "Building the application..."
go build .

./cats-of-asia-reloaded prepare-local

cleanup() {
  docker compose down
}
trap cleanup EXIT
trap 'exit 130' INT TERM

if ! docker compose up --wait --wait-timeout 120; then
  echo "Development services failed to start or become ready within 120 seconds." >&2
  docker compose logs --no-color >&2 || true
  exit 1
fi

docker compose exec -T db sh -c \
  'exec psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' \
  < scripts/schema.sql

./cats-of-asia-reloaded "$@"
