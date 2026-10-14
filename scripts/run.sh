#!/usr/bin/env bash

set -e

set -a
source .env
set +a

echo "Building the application..."
go build .

cleanup() {
  docker compose down
}
trap cleanup EXIT
trap 'exit 130' INT TERM

cat > pgconsole.toml <<EOF
[[connections]]
id = "cats"
name = "Cats of Asia local"
host = "db"
port = 5432
database = "${POSTGRES_DB}"
username = "${POSTGRES_USER}"
password = "${POSTGRES_PASSWORD}"
EOF

if ! docker compose up --wait --wait-timeout 120; then
  echo "Development services failed to start or become ready within 120 seconds." >&2
  docker compose logs --no-color >&2 || true
  exit 1
fi

docker compose exec -T db psql \
  -v ON_ERROR_STOP=1 \
  -U "$POSTGRES_USER" \
  -d "$POSTGRES_DB" < scripts/schema.sql

./cats-of-asia-reloaded "$@"
