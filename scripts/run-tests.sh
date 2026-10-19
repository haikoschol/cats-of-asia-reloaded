#!/usr/bin/env bash

set -euo pipefail

echo "Building the application..."
go build .

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

for SQL_FILE in scripts/schema.sql scripts/test-data.sql; do
  test_compose exec -T db sh -c \
    'exec psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' \
    < "$SQL_FILE"
done

./cats-of-asia-reloaded run-tests "$TEST_PORT"
