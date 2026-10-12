#!/usr/bin/env bash

set -e

set -a
source .env
set +a

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

until docker compose exec -T db pg_isready \
  -U "$POSTGRES_USER" \
  -d "$POSTGRES_DB" >/dev/null; do
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

exec ./cats-of-asia-reloaded "$@"
