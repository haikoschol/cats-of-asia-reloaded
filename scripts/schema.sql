CREATE EXTENSION IF NOT EXISTS postgis;

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
