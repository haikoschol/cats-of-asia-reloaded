# Cats of Asia Reloaded

A small Go web application backed by PostgreSQL with PostGIS. The homepage lets
you select a place and displays its name in the center of the page.

## Prerequisites

- Go 1.27 or later
- Docker with Docker Compose v2 supporting `up --wait --wait-timeout`

The database uses PostgreSQL 18 with PostGIS 3.6. Its image is AMD64-only, so
Compose explicitly selects `linux/amd64`. On Apple Silicon Macs, Docker Desktop
must support running AMD64 containers through emulation.

## Setup and run

Clone the repository and change into the project directory:

```bash
git clone git@github.com:haikoschol/cats-of-asia-reloaded.git
cd cats-of-asia-reloaded
```

Create the local database configuration:

```bash
cp .env.example .env
```

Set the PostgreSQL values in `.env`. `DATABASE_URL` is assembled from these
values, so the connection settings cannot get out of sync. The development
database includes PostGIS, which is activated automatically when the schema is
created.

Start the database and the web application:

```bash
./scripts/run.sh
```

The script runs `go build .` before starting the containers and application, so
each run uses the current source code. If compilation fails, startup stops.
This also applies when using the script to create a user account.

Open [http://localhost:3000/](http://localhost:3000/) in a browser.

The header shows **Cats of Asia** and a **Places** menu populated from
`photos_location`. Click **Places** to open the list, then select a place to
display its city and country in the center of the page without reloading it.
The menu closes after selection and its label remains **Places**. Before a
selection, the page displays **No place selected**. If there are no places, the
menu contains an empty list. The three example places are available automatically
after startup; additional database entries also appear in the menu.

Selecting a place stores its database ID in the URL, for example `/?place=1`.
Reloading the page or opening that link restores the selection, provided the
place exists in that database. The browser's Back and Forward buttons restore
earlier selections. Missing or unknown IDs display **No place selected**.

Press `Ctrl+C` to stop the web application and remove the database and pgconsole
containers. Database contents remain in the `postgres18-data` Docker volume.
The containers are also cleaned up when a command exits or startup fails.

Both start scripts wait up to 120 seconds for services to become ready after
container startup. The database healthcheck connects over TCP, so the temporary
initialization server cannot report readiness too early. Startup failures print
container logs before cleanup.

### Existing PostgreSQL 17 databases

PostgreSQL 18 uses a new volume, `postgres18-data`, mounted at
`/var/lib/postgresql`. The previous `postgres-data` volume is preserved, but its
users and other data are **not automatically imported**. Without an import, the
app starts with a fresh database and accounts must be created again.

To retain your existing accounts and data, export the PostgreSQL 17 database
and restore it before using the new database. The following commands assume the
default Compose project name `cats-of-asia-reloaded` and unchanged `.env`
credentials. If you used another project name, use the corresponding volume
name from `docker volume ls`. Run these commands from the repository root.

Stop the development environment, load its connection settings, and verify the
old volume exists (do not continue if `inspect` fails):

```bash
docker compose down
set -a
source .env
set +a
docker volume inspect cats-of-asia-reloaded_postgres-data
```

Start the old database without exposing a host port, then export it. Store the
backup outside the repository because it contains user data and password hashes:

```bash
docker run --detach --rm --name cats-postgres17 --platform linux/amd64 \
  --mount type=volume,source=cats-of-asia-reloaded_postgres-data,target=/var/lib/postgresql/data \
  postgis/postgis:17-3.5
```

Wait until `docker logs cats-postgres17` reports that the database is ready to
accept connections. Then run:

```bash
docker exec cats-postgres17 pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  --format=custom > ../cats-postgres17.dump
```

Only continue if the export succeeds. Stop the old container and start the new
database:

```bash
docker stop cats-postgres17
docker compose up --wait --wait-timeout 120 db
```

The restore below replaces objects in the **new PostgreSQL 18 database**. Use it
before creating new data there, or back up that new data first:

```bash
docker compose exec -T db pg_restore --exit-on-error --single-transaction \
  --clean --if-exists --no-owner --no-privileges \
  -U "$POSTGRES_USER" -d "$POSTGRES_DB" < ../cats-postgres17.dump
docker compose down
./scripts/run.sh
```

Keep the old volume and the backup until you have verified your data. Neither
the normal start script nor the test script deletes the old development volume.

## Configuration for image and map services

`config.go` reads the application's settings from environment variables once at
startup. `scripts/run.sh` loads and exports your local `.env` file before starting
the app. When running the executable directly, export its settings yourself;
the executable does not read `.env` files.

`.env.example` contains harmless example values. If you already have a `.env`
file, add the new settings to it as needed instead of replacing your database
credentials. Keep real credentials in your ignored `.env` file or the server's
environment, not in `.env.example`.

| Setting | Purpose | Requirement |
| --- | --- | --- |
| `DATABASE_URL` | PostgreSQL connection, including credentials; server-only | Required for every app command |
| `CLOUDFLARE_IMAGES_DOMAIN` | Public image-delivery hostname, without a scheme or path | Required when displaying images |
| `CLOUDFLARE_IMAGES_ACCOUNT_HASH` | Public account identifier used in image URLs | Required when displaying images |
| `MAPBOX_ACCESS_TOKEN` | Public browser token for fetching map tiles | Required when displaying a map |

The current homepage does not display images or a map. Their settings are
loaded but optional at this stage; `validateImages` and `validateMap` provide
the checks for the future feature handlers. Creating user accounts only needs
the database connection. Missing required settings produce a message naming
the variable, such as `DATABASE_URL is not set`, without including its value.

The image domain and account hash identify public image URLs; neither grants
upload access. The map setting must be a token intended for use in a browser.
These three values can be passed to HTML or JSON when the image and map features
are implemented. The dummy values in `.env.example` are sufficient for local
development and configuration tests, but cannot load real Cloudflare images or
Mapbox tiles. Replace them with the corresponding public settings when those
features are needed.

Future uploads, server-side geocoding, and Mastodon posting use separate private
settings: `CLOUDFLARE_IMAGES_ACCOUNT_ID`, `CLOUDFLARE_IMAGES_API_KEY`,
`GOOGLE_MAPS_API_KEY`, and `MASTODON_ACCESS_TOKEN`. They are not read or required
by the current Go app. Keep them on the server and never pass them to templates,
JavaScript, JSON responses, or logs. Do not log the complete configuration or
the credential-bearing database URL.

## User accounts

Create a user account with a username as the only argument:

```bash
./scripts/run.sh create-user peter
```

The command prompts twice for a password without displaying the characters you
type. It creates the account in the local PostgreSQL database and then exits.

Start the web application with `./scripts/run.sh`, then open
[http://localhost:3000/login](http://localhost:3000/login) to sign in. A
successful login redirects to the homepage. Use
the **Log out** button in the top-right corner to end the session.

## Run tests

Run the automated homepage test with:

```bash
./scripts/run-tests.sh
```

The script starts a separate PostgreSQL test database on port `5433`, applies
the shared schema with its example places, and runs `go test -count=1 ./...`
without reusing cached test results. It checks that the homepage returns HTTP
status `200` and displays `No place selected`. The test database, its Docker
volume, and its network are removed automatically when the command finishes.
Schema tests also check inserting related photo data in dependency order,
geographic coordinates, cascading deletions, retaining photos without
coordinates, and reapplying the schema without losing data.

Configuration tests set their own example environment values and check missing
settings and feature-specific requirements. Regression tests use fake private
credentials to check that homepage HTML and database-configuration startup error
logs do not expose them. They do not contact Cloudflare or Mapbox and do not
require real service credentials.

## Inspecting the local database

Both `scripts/run.sh` and `scripts/run-tests.sh` apply `scripts/schema.sql`.
It includes six application tables based on a schema-only export from the
production PostgreSQL 16.9 database on 2026-10-13. Table names, column names,
types, and identity sequences remain compatible with its data. The read-only
export used:

```bash
pg_dump --schema-only --no-owner --no-privileges --table='public.photos_*'
```

The export contains structure only, not rows or connection credentials. The
following tables are included:

| Table | Contents |
| --- | --- |
| `photos_location` | City, country, and time-zone offset |
| `photos_coordinates` | Geographic point, altitude, and reference to a location |
| `photos_photo` | Photo UUID, filename, unique SHA-256 hash, timestamp, and optional coordinates |
| `photos_rawmetadata` | Raw JSON metadata linked to a photo's SHA-256 hash |
| `photos_platform` | Social platform name and profile URL |
| `photos_post` | Publication time and references to a photo's hash and a platform |

Coordinates use PostGIS `geography(Point, 4326)` with a spatial GiST index.
Points are supplied as longitude first, latitude second. Unique constraints
prevent duplicate city/country pairs, point/altitude pairs, and photo/platform
pairs. Foreign keys check references immediately. PostgreSQL handles deletion
rules directly: deleting a location deletes its coordinates, but retains its
photos with NULL coordinates; deleting a photo deletes its metadata and posts;
deleting a platform deletes its posts. Indexes use descriptive names and cover
foreign keys and geographic searches. Django-generated pattern-search indexes
and deferred foreign-key checks are omitted.

For a future production-data import, use a **data-only** export of these six
tables, not a schema restore. Insert locations and platforms first, then
coordinates, photos, and finally metadata and posts. Retain exported IDs, hashes,
and timestamps, and restore the identity-sequence values so subsequently created
rows do not reuse imported IDs. A PostgreSQL data-only dump contains sequence
values as well as rows. Values normally supplied by Django (including photo
UUIDs, points, and timestamps) must be provided by the caller when inserting rows.

The existing Go `users` and `sessions` tables remain in use.
Django-specific authentication, admin, session, and migration bookkeeping tables
are not copied. The schema also seeds three example places read from production:
Bali (Indonesia), Vientiane (Laos), and Kuala Lumpur (Malaysia), with their
original `tzoffset` values unchanged. Existing city/country pairs are skipped,
so repeated starts create no duplicates and do not overwrite edited places.
Photos and platforms are not seeded; this is not a full production-data import.

The schema removes the obsolete `greetings` demo table, including its example
rows, on the next start. Users, sessions, and photo data are preserved; no
database reset is needed for this change.

Re-running either script preserves the remaining tables and data. `CREATE TABLE
IF NOT EXISTS` does not change previously created tables. If you used an earlier
development version of this schema, reset your local database as described
below so the simplified constraints take effect.

### Resetting the local development database

Stop `scripts/run.sh` first. The following command deletes this Compose project's
development database volume, including all local users, sessions, and photo
data. Back up any data you want to retain before running it. It does not affect
Railway's production database or the old PostgreSQL 17 volume.

```bash
docker compose down --volumes
./scripts/run.sh
```

The next start creates a fresh database with the current schema and example
places. Recreate local accounts using the user creation command above. The
test script already creates and removes its own database on every run.

While `./scripts/run.sh` is running, open a second terminal in the repository
root to list the imported tables with `psql`:

```bash
docker compose exec db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "\dt public.photos_*"'
```

To inspect columns, constraints, and indexes, for example for coordinates:

```bash
docker compose exec db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "\d public.photos_coordinates"'
```

`./scripts/run.sh` also starts pgconsole, a browser-based PostgreSQL client.

Open [http://localhost:3001/](http://localhost:3001/) in a browser. No separate
login is required; pgconsole is only available on your local computer.

The `photos_location` table is listed in the sidebar. Selecting it shows its schema.
To view the rows, click **New Query** and run:

```sql
SELECT * FROM photos_location;
```

The query result displays the example places. Values can be edited directly
in the result grid and pgconsole will prepare the corresponding SQL update.

## Stop the database

```bash
docker compose down
```
