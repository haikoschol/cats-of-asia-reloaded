#!/usr/bin/env bash

set -e

set -a
source .env
set +a

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

docker compose up -d

database_is_ready() {
  docker compose exec -T db sh -c \
    'case "$(tr "\000" " " < /proc/1/cmdline)" in postgres*) exit 0;; *) exit 1;; esac' &&
    docker compose exec -T db psql \
      -v ON_ERROR_STOP=1 \
      -U "$POSTGRES_USER" \
      -d "$POSTGRES_DB" \
      -c 'SELECT 1' >/dev/null 2>&1
}

until database_is_ready; do
  sleep 1
done

docker compose exec -T db psql \
  -v ON_ERROR_STOP=1 \
  -U "$POSTGRES_USER" \
  -d "$POSTGRES_DB" < scripts/schema.sql

docker compose exec -T db psql \
  -v ON_ERROR_STOP=1 \
  -U "$POSTGRES_USER" \
  -d "$POSTGRES_DB" <<'SQL'
INSERT INTO greetings (greeting)
VALUES
    ('hello'),
    ('moin'),
    ('tach'),
    ('salve')
ON CONFLICT (greeting) DO NOTHING;
SQL

./cats-of-asia-reloaded "$@"
