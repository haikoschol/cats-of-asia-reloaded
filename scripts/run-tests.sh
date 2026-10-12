#!/usr/bin/env bash

set -euo pipefail

set -a
source .env
set +a

TEST_PROJECT="cats-of-asia-reloaded-test"
TEST_PORT=5433

test_compose() {
  POSTGRES_HOST_PORT="$TEST_PORT" docker compose \
    --project-name "$TEST_PROJECT" "$@"
}

cleanup() {
  test_compose down --volumes --remove-orphans
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

if ! test_compose up --wait --wait-timeout 120 db; then
  echo "Test database failed to start or become ready within 120 seconds." >&2
  test_compose logs db >&2 || true
  exit 1
fi

test_compose exec -T db psql \
  -v ON_ERROR_STOP=1 \
  -U "$POSTGRES_USER" \
  -d "$POSTGRES_DB" < scripts/schema.sql

test_compose exec -T db psql \
  -v ON_ERROR_STOP=1 \
  -U "$POSTGRES_USER" \
  -d "$POSTGRES_DB" <<'SQL'
TRUNCATE greetings RESTART IDENTITY;

INSERT INTO greetings (greeting)
VALUES
    ('hello'),
    ('moin'),
    ('tach'),
    ('salve');
SQL

export DATABASE_URL="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@localhost:${TEST_PORT}/${POSTGRES_DB}?sslmode=disable"

go test -count=1 ./...
