#!/usr/bin/env bash

set -euo pipefail

if [[ -z "${DEVENV_RUNTIME:-}" || -z "${PGPORT:-}" ]]; then
  echo "Start development with devenv up." >&2
  exit 1
fi

socket="$DEVENV_RUNTIME/postgres"
role=$(id -un)
unset PGSERVICE PGHOSTADDR PGOPTIONS PGTARGETSESSIONATTRS
if (( $# == 0 )); then
  go build .
  psql -X -w -v ON_ERROR_STOP=1 -h "$socket" -p "$PGPORT" -U "$role" -d cats -f scripts/schema.sql
fi
socket=$(jq -nr --arg socket "$socket" '$socket | @uri')
role=$(jq -nr --arg role "$role" '$role | @uri')
export USERS_DATABASE_URL="postgresql:///cats?host=$socket&port=$PGPORT&user=$role&sslmode=disable"
unset PGHOST PGPORT
exec ./cats-of-asia-reloaded "$@"
