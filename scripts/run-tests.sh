#!/usr/bin/env bash

set -euo pipefail

suite="${1:-all}"
if (( $# > 1 )); then
  echo "Usage: $0 [unit|integration|all]" >&2
  exit 2
fi
case "$suite" in
  unit|integration|all) ;;
  *) echo "Usage: $0 [unit|integration|all]" >&2; exit 2 ;;
esac

if [[ "$suite" != integration ]]; then
  go test -count=1 ./...
fi

if [[ "$suite" == unit ]]; then
  exit 0
fi

node scripts/upload-metadata.test.cjs
node scripts/upload-transfer.test.cjs
node scripts/map.test.cjs

work_dir=$(mktemp -d /tmp/coa-test.XXXXXXXX)
data_dir="$work_dir/data"
port=5433
role=$(id -un)

cleanup() {
  if [[ -f "$data_dir/postmaster.pid" ]]; then
    pg_ctl -D "$data_dir" -m fast -w stop || return
  fi
  rm -rf -- "$work_dir"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

initdb -D "$data_dir" -U "$role" --locale=C --encoding=UTF8 \
  --auth-local=peer --auth-host=reject > "$work_dir/init.log" || {
  cat "$work_dir/init.log" >&2
  exit 1
}
pg_ctl -D "$data_dir" -l "$work_dir/postgres.log" -w start \
  -o "-c listen_addresses='' -k '$work_dir' -p $port" || {
  cat "$work_dir/postgres.log" >&2
  exit 1
}

unset PGSERVICE PGHOSTADDR PGOPTIONS PGTARGETSESSIONATTRS
export PGUSER="$role"
createdb -w -h "$work_dir" -p "$port" cats_test
psql -X -w -v ON_ERROR_STOP=1 -h "$work_dir" -p "$port" -d cats_test \
  -f scripts/schema.sql -f scripts/test-data.sql

role=$(jq -nr --arg role "$role" '$role | @uri')
export CATS_TEST_DATABASE_URL="postgresql:///cats_test?host=$work_dir&port=$port&user=$role&sslmode=disable"
export DATABASE_URL="$CATS_TEST_DATABASE_URL"
export USERS_DATABASE_URL="$CATS_TEST_DATABASE_URL"
export PLAUSIBLE_ENABLED=false
go test -count=1 -tags integration ./...
