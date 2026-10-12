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

database_is_ready() {
  test_compose exec -T db sh -c \
    'case "$(tr "\000" " " < /proc/1/cmdline)" in postgres*) exit 0;; *) exit 1;; esac' &&
    test_compose exec -T db psql \
      -v ON_ERROR_STOP=1 \
      -U "$POSTGRES_USER" \
      -d "$POSTGRES_DB" \
      -c 'SELECT 1' >/dev/null 2>&1
}

cleanup() {
  test_compose down --volumes --remove-orphans
}
trap cleanup EXIT

test_compose up --detach db

for _ in {1..30}; do
  if database_is_ready; then
    break
  fi
  sleep 1
done

if ! database_is_ready; then
  echo "Test database '$POSTGRES_DB' was not ready after 30 seconds." >&2
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

go test ./...
