#!/usr/bin/env bash

set -euo pipefail

set -a
source .env
set +a

test_project="cats-of-asia-reloaded-test"
test_port=5433

cleanup() {
  POSTGRES_HOST_PORT="$test_port" docker compose \
    --project-name "$test_project" \
    down --volumes --remove-orphans
}
trap cleanup EXIT

POSTGRES_HOST_PORT="$test_port" docker compose \
  --project-name "$test_project" \
  up --detach db

until docker compose --project-name "$test_project" exec -T db pg_isready \
  -U "$POSTGRES_USER" \
  -d "$POSTGRES_DB" >/dev/null; do
  sleep 1
done

docker compose --project-name "$test_project" exec -T db psql \
  -v ON_ERROR_STOP=1 \
  -U "$POSTGRES_USER" \
  -d "$POSTGRES_DB" <<'SQL'
CREATE TABLE IF NOT EXISTS greetings (
    id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    greeting varchar(255) NOT NULL UNIQUE
);

TRUNCATE greetings RESTART IDENTITY;

INSERT INTO greetings (greeting)
VALUES
    ('hello'),
    ('moin'),
    ('tach'),
    ('salve');
SQL

export DATABASE_URL="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@localhost:${test_port}/${POSTGRES_DB}?sslmode=disable"

go test ./...
