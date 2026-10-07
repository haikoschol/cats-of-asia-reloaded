#!/usr/bin/env bash

set -e

set -a
source .env
set +a

docker compose up -d

until docker compose exec -T db pg_isready \
  -U "$POSTGRES_USER" \
  -d "$POSTGRES_DB" >/dev/null; do
  sleep 1
done

docker compose exec -T db psql \
  -v ON_ERROR_STOP=1 \
  -U "$POSTGRES_USER" \
  -d "$POSTGRES_DB" <<'SQL'
CREATE TABLE IF NOT EXISTS greetings (
    id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    greeting varchar(255) NOT NULL UNIQUE
);

INSERT INTO greetings (greeting)
VALUES
    ('hello'),
    ('moin'),
    ('tach'),
    ('salve')
ON CONFLICT (greeting) DO NOTHING;
SQL

exec ./cats-of-asia-reloaded