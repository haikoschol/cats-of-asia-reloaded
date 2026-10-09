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
  -d "$POSTGRES_DB" <<'SQL'
CREATE TABLE IF NOT EXISTS greetings (
    id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    greeting varchar(255) NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS users (
    id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username varchar(255) NOT NULL UNIQUE,
    hashed_password char(60) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sessions (
    token text PRIMARY KEY,
    data bytea NOT NULL,
    expiry timestamptz NOT NULL
);

CREATE INDEX IF NOT EXISTS sessions_expiry_idx ON sessions (expiry);

INSERT INTO greetings (greeting)
VALUES
    ('hello'),
    ('moin'),
    ('tach'),
    ('salve')
ON CONFLICT (greeting) DO NOTHING;
SQL

exec ./cats-of-asia-reloaded "$@"
