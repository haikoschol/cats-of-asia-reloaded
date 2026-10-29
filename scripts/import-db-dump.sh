#!/usr/bin/env bash

set -euo pipefail

usage() {
  echo "Usage: $0 [-replace] path/to/backup.dump"
  echo "Import photo data into the running local Docker Compose database."
  echo "-replace clears photo tables first; users and sessions are preserved."
}

replace=false
dump_file=
for arg in "$@"; do
  case "$arg" in
    -replace) replace=true ;;
    -h|--help) usage; exit 0 ;;
    -*) usage >&2; exit 1 ;;
    *)
      if [[ -n "$dump_file" ]]; then usage >&2; exit 1; fi
      dump_file=$arg
      ;;
  esac
done

if [[ -z "$dump_file" || ! -f "$dump_file" || ! -r "$dump_file" ]]; then
  echo "Provide a readable PostgreSQL archive dump file." >&2
  usage >&2
  exit 1
fi

# Resolve the input before changing directory, so paths are relative to the caller.
dump_file="$(cd -- "$(dirname -- "$dump_file")" && pwd)/$(basename -- "$dump_file")"
project_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd -- "$project_dir"

work_dir=$(mktemp -d)
container_dir=
cleanup() {
  if [[ -n "$container_dir" ]]; then
    docker compose exec -T db rm -rf -- "$container_dir" >/dev/null 2>&1 || true
  fi
  rm -rf -- "$work_dir"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

container_dir=$(docker compose exec -T db mktemp -d /tmp/import-db-dump.XXXXXXXX)
docker compose cp "$dump_file" "db:$container_dir/backup.dump" >/dev/null
docker compose exec -T db pg_restore --list "$container_dir/backup.dump" > "$work_dir/archive.list"

# Restore only the application's content tables, in foreign-key dependency order.
# Include their sequence values, while excluding authentication and Django tables.
awk '
  BEGIN {
    split("photos_location photos_platform photos_coordinates photos_photo photos_rawmetadata photos_post", tables)
    for (i = 1; i <= 6; i++) rank[tables[i]] = i
  }
  $4 == "TABLE" && $5 == "DATA" && $6 == "public" && ($7 in rank) {
    rows[rank[$7]] = rows[rank[$7]] $0 "\n"
    found++
  }
  $4 == "SEQUENCE" && $5 == "SET" && $6 == "public" {
    for (i = 1; i <= 6; i++) {
      if ($7 == tables[i] "_id_seq") sequences = sequences $0 "\n"
    }
  }
  END {
    if (!found) exit 1
    for (i = 1; i <= 6; i++) printf "%s", rows[i]
    printf "%s", sequences
  }
' "$work_dir/archive.list" > "$work_dir/import.list" || {
  echo "The dump contains no public photo-table data to import." >&2
  exit 1
}

docker compose cp "$work_dir/import.list" "db:$container_dir/import.list" >/dev/null

# Generate the complete import before touching the database. psql rolls back
# table changes if schema initialization, truncation, or data loading fails.
printf 'BEGIN;\n' > "$work_dir/import.sql"
cat scripts/schema.sql >> "$work_dir/import.sql"
if "$replace"; then
  cat >> "$work_dir/import.sql" <<'SQL'
TRUNCATE TABLE public.photos_post, public.photos_rawmetadata,
  public.photos_photo, public.photos_coordinates, public.photos_platform,
  public.photos_location RESTART IDENTITY;
SQL
fi
docker compose exec -T db pg_restore \
  --data-only --no-owner --no-privileges --file=- \
  --use-list="$container_dir/import.list" \
  "$container_dir/backup.dump" >> "$work_dir/import.sql"
printf '\nCOMMIT;\n' >> "$work_dir/import.sql"

docker compose exec -T db sh -c \
  'exec psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' \
  < "$work_dir/import.sql"

echo "Photo data imported. Local users and sessions were preserved."
