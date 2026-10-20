# Cats of Asia Reloaded

A small Go web application backed by PostgreSQL with PostGIS. The homepage lets
you select a place and displays geolocated photos as red circles on a map.

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

Set the PostgreSQL values in `.env` for the local users/session container.
Both `DATABASE_URL` and `USERS_DATABASE_URL` must be set explicitly:

- `DATABASE_URL` points to an existing database with the Django photo schema.
  Ask the database owner for access; the placeholder in `.env.example` does not
  connect to a real server. Photos **and the Places menu** use this connection,
  which the Go app opens read-only.
- `USERS_DATABASE_URL` points to the local Docker database, normally on port
  `5432`, and must match `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB`.
  Only users and sessions use this connection.

The development container includes PostGIS; the startup schema is applied only
inside that local container, never to the external content database.

Start the database and the web application:

```bash
./scripts/run.sh
```

The script runs `go build .` before starting the containers and application, so
each run uses the current source code. If compilation fails, startup stops.
This also applies when using the script to create a user account.
The script uses the Go application's `prepare-local` command to generate the
ignored `pgconsole.toml` file with owner-only permissions. Docker Compose reads
its container settings from `.env`; the script does not source the file.

Open [http://localhost:3000/](http://localhost:3000/) in a browser.

The header shows **Cats of Asia** and a **Places** menu populated from
`photos_location`. Click **Places** to open the list and select a place without
reloading the homepage. The menu closes after selection and its label shows
the selected city and country. The selected-place heading is retained for assistive technology,
but is not visually displayed above the map. If there are no places, the
menu contains an empty list. The list comes from `DATABASE_URL`, just like the
photos; local photo tables do not feed the menu. Normal startup does not create
example places.

Selecting a place stores its database ID in the URL, for example `/?place=1`.
Reloading the page or opening that link restores the selection, provided the
place exists in that database. The browser's Back and Forward buttons restore
earlier selections and move the map accordingly. Missing or unknown IDs leave
no place selected, reset the menu label to **Places**, and show the map overview.

Selecting a place centres the map on its first photo with valid coordinates,
in photo API order, at zoom level 14. Selection waits for the photo request to
finish, including when following a link or changing places while data loads.
If the place has no matching geolocated photos, a visible notice appears and
the map does not jump to an unrelated location. Places and photos are matched
directly by the shared place ID (`location_id` in the photo API).

The homepage and login page share `template/base.tmpl` and the header. The
Places menu is also available on the login page; selecting a place there opens
the homepage with that selection. Escape closes the menu and returns focus to
its toggle; clicking outside also closes it.

Go serves the shared stylesheet, place-selector JavaScript, SVG site icon, and
vendored frontend assets from `static/` under `/static/`. Missing static files
return 404. No CDN, package installation, or separate frontend build is required
to run the app. Run the app from the repository root so both
`template/` and `static/` can be found. Changes to CSS or JavaScript do not require
recompiling Go; reload the browser (bypassing its cache if needed).

The homepage additionally loads the pinned Leaflet 1.9.4 JavaScript and CSS from
`static/vendor/leaflet/`; the login page does not load Leaflet. The upstream
BSD-2-Clause license, asset provenance, and integrity values are retained in
[the vendored Leaflet directory](static/vendor/leaflet/README.md).

### Homepage map

Set `MAPBOX_ACCESS_TOKEN` in your ignored `.env` to a working **public** Mapbox
browser token starting with `pk.`, then start or restart `./scripts/run.sh`.
Open [http://localhost:3000](http://localhost:3000) to see the map. The browser
loads `/api/photos` and places a red circle at each distinct point with valid
latitude and longitude. Without a valid place selection, the initial view fits all markers,
with a maximum initial zoom of 14
so a single photo does not zoom in too far. If there are no photos with valid
coordinates, the map stays centered on Southeast Asia and displays a message.
Photos without valid coordinates are skipped. The normal view shows only the
header and the map, without a selected-place heading, instructions, or photo
count above it. There is no divider line below the header, and the map fills
the remaining window height. Drag to pan and use the +/− buttons or mouse wheel
to zoom. The browser fetches Mapbox Streets v12 raster tiles directly; internet
access is needed. Tile requests count towards the Mapbox account's usage.
If the token has URL restrictions, allow `http://localhost:3000` for local work
and the actual site URL for deployment. Never use a secret `sk.` token here.

Missing configuration, tile-loading failures, and photo API failures produce
visible notices over the map. Empty photo results also show a notice. Tile
and photo messages are separate so one
cannot hide the other's errors. Photo requests time out after 15 seconds;
reload the page to retry a failed request. The rest of the page and login
remain available. The map includes
Leaflet, Mapbox, and OpenStreetMap attribution, an Improve this map link, and
the official Mapbox logo. Keep these visible when changing the layout.

Click a photo marker to open a preview with its place and capture time (shown
explicitly in UTC). Click the preview to open the larger `desktop` image in a
full-window overlay in the same tab. Close it using the **×** button at the
top right or **Escape** to return to the unchanged map and popup. The larger
image is also the preview's link target: use the browser's right-click menu or
Ctrl/Cmd-click to open it in a separate tab. An unmodified click keeps using the
overlay. The larger image is requested only when opening it; loading failures show a message
without hiding the close button. The `smol` preview is requested only when that
photo is selected, not when loading the map. Failed previews display a message
and retain a link for opening the larger image. Place names are rendered as
text, never as HTML. Popup width
and image height adapt to narrow screens; tall content can scroll.

Photos with identical coordinates share one marker. Clicking it fans out a
separate circle for every photo, with lines to the original point. Select any
circle to open that photo. Clicking the original group again or the map
background collapses it, as do dragging, zooming, resizing, or selecting a place.
Automatic panning to fit a popup keeps the group open. Only temporary display
markers are offset; original photo coordinates remain unchanged.

Selecting a place updates the menu, URL, screen-reader heading, and map view.
Opening the map reads the configured photo source through the existing
read-only API; it does not import or modify photo records.

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

`config.go` loads the optional `.env` file from the current working directory
using [godotenv](https://github.com/joho/godotenv), merges it with the process
environment, and parses the result using
[caarlos0/env](https://github.com/caarlos0/env). The file is read as data, not
executed as shell commands. Loading configuration does not change the parent
process's environment.

Explicit environment variables take precedence over `.env`, including empty
values. A missing file is allowed when settings are supplied by the deployment
environment. An unreadable or malformed file stops startup with a generic error
that does not include its contents.

For direct execution, build the app and run it from the repository root while
the users database is already running:

```bash
go build .
./cats-of-asia-reloaded
```

The executable reads `.env` itself; no shell export step is needed. On a server,
provide settings through the service environment instead. `scripts/run.sh`
remains the convenient development command that also starts the containers.

`.env.example` contains harmless example values. If you already have a `.env`
file, add the new settings to it as needed instead of replacing your database
credentials. Keep real credentials in your ignored `.env` file or the server's
environment, not in `.env.example`.

| Setting | Purpose | Requirement |
| --- | --- | --- |
| `DATABASE_URL` | Read-only content connection for photos, coordinates, and locations; server-only | Required explicitly; use TLS for a remote source |
| `USERS_DATABASE_URL` | Read/write connection for users and sessions; server-only | Required explicitly; normally the local Docker database |
| `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | Local Docker database and pgconsole credentials; server-only | Required by the development and test scripts |
| `CLOUDFLARE_IMAGES_DOMAIN` | Public image-delivery hostname, without a scheme or path | Required when the photo API returns photos |
| `CLOUDFLARE_IMAGES_ACCOUNT_HASH` | Public account identifier used in image URLs | Required when the photo API returns photos |
| `MAPBOX_ACCESS_TOKEN` | Public browser token for fetching map tiles | Required when displaying a map |

The homepage displays a map with photo markers when a public Mapbox token is
configured; it does not display photo images yet. The photo API checks the
image domain and account hash
when photos exist; an empty result needs neither. Creating user
accounts uses only the users connection, although both URLs must be configured. Missing required settings produce
a message naming the variable, such as `DATABASE_URL is not set`, without
including its value.

Neither URL is derived automatically from the Docker settings. Explicit URLs
can use `${POSTGRES_USER}`, `${POSTGRES_PASSWORD}`, and `${POSTGRES_DB}`
references in `.env`; URL-encode special characters in URI credentials.

### Migrating an existing local configuration

The meaning of `DATABASE_URL` has changed. Stop the old app before editing
`.env`, then make these changes together with the updated code:

1. Rename the old local `DATABASE_URL` setting to `USERS_DATABASE_URL`.
2. Rename the old external `PHOTOS_DATABASE_URL` setting to `DATABASE_URL`.
3. Remove `PHOTOS_DATABASE_URL`; it is no longer read and there is no fallback.

Keep the connection values and Docker credentials unchanged so existing local
accounts and sessions remain available. Update any exported environment values
as well: they still override `.env`. If you previously used one local database
for everything, both required URLs may point to that database. No tables or
records are copied or deleted by this configuration change.

Place IDs in links now refer to the content database. Old local `?place=...`
links may identify a different place or no place; select it again from the menu.

The image domain and account hash identify public image URLs; neither grants
upload access. The map setting must be a token intended for use in a browser.
These three values can be passed to HTML or JSON when the image and map features
need them; the photo API already uses the image settings to build its URLs.
The dummy values in `.env.example` are sufficient for local
development and configuration tests, but cannot load real Cloudflare images or
Mapbox tiles. Replace them with the corresponding public settings when those
features are needed.

Future uploads, server-side geocoding, and Mastodon posting use separate private
settings: `CLOUDFLARE_IMAGES_ACCOUNT_ID`, `CLOUDFLARE_IMAGES_API_KEY`,
`GOOGLE_MAPS_API_KEY`, and `MASTODON_ACCESS_TOKEN`. They are not read or required
by the current Go app. Keep them on the server and never pass them to templates,
JavaScript, JSON responses, or logs. Do not log the complete configuration or
the credential-bearing database URL.

## Photo API

While the local app is running, open
[http://localhost:3000/api/photos](http://localhost:3000/api/photos) or run:

```bash
curl http://localhost:3000/api/photos
```

`GET /api/photos` returns a public JSON array of photos from the configured
`DATABASE_URL`, the same database used by the Places menu. Results are ordered by capture timestamp and then UUID. An empty
photo database returns `[]`, not `null`. Each entry contains only the fields shown in this
example (the image settings and photo identifiers below are example values):

```json
[
  {
    "id": "21000000-0000-4000-8000-000000000001",
    "sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "timestamp": "2026-01-02T03:30:00Z",
    "location_id": 1,
    "latitude": 13.75,
    "longitude": 100.5,
    "city": "Bangkok",
    "country": "Thailand",
    "urls": {
      "desktop": "https://images.example.com/cdn-cgi/imagedelivery/example-account-hash/21000000-0000-4000-8000-000000000001/desktop",
      "mobile": "https://images.example.com/cdn-cgi/imagedelivery/example-account-hash/21000000-0000-4000-8000-000000000001/mobile",
      "smol": "https://images.example.com/cdn-cgi/imagedelivery/example-account-hash/21000000-0000-4000-8000-000000000001/smol"
    }
  }
]
```

Timestamps use ISO 8601/RFC 3339 in UTC (`Z`). PostGIS X is longitude and Y is
latitude. `location_id` is the numeric ID of the associated place in the same
content database used by the Places menu; it does not replace the photo UUID in
`id`. A photo without coordinates remains in the result, with `location_id`,
`latitude`, `longitude`, `city`, and `country` explicitly set to `null`. Known empty city or
country strings remain empty strings. All three image URLs use the same
centrally constructed Cloudflare base URL and the photo UUID.

The query runs in a read-only transaction and does not import, create, or change
photo records. Filenames, raw EXIF metadata, and credentials are not returned.
Other HTTP methods receive `405 Method Not Allowed`. Database, configuration,
or JSON errors return a generic `500`; missing image-setting names are recorded
only in the server log. Mapbox settings are not needed for this endpoint.

Set `DATABASE_URL` in your ignored `.env` using the content database details
provided by the owner. Keep `USERS_DATABASE_URL` pointing to the separate users
database; the content connection is never used for account creation or sessions.

Every content connection starts with `default_transaction_read_only=on`,
queries have a ten-second statement timeout, and photo queries additionally
use a read-only transaction. Photo and location reads have twelve-second context
deadlines, including pool waits. Photo transactions end before image URLs are
constructed. A dedicated read-only database role is preferable; these application
settings do not replace database access permissions.

The content connection is established lazily on the first location/photo read.
An unavailable source now also affects homepage/login rendering because both
pages load its Places menu. Creating a user account only connects to
`USERS_DATABASE_URL`, so it does not need the content server to be reachable.

Use the source deployment's public `CLOUDFLARE_IMAGES_DOMAIN` and
`CLOUDFLARE_IMAGES_ACCOUNT_HASH` for image URLs. No private Cloudflare upload
key or Mapbox token is needed for the API. No production data is copied locally;
pgconsole still shows the local Docker database. Existing local photo/place
tables may remain, but the app does not use them when content is external.
The startup scripts apply schema only inside the local Docker container, not
through `DATABASE_URL`. Never run migrations or seeds against the original
database, and do not commit either real connection URL.

## User accounts

Create a user account with a username as the only argument:

```bash
./scripts/run.sh create-user peter
```

The command prompts twice for a password without displaying the characters you
type. It creates the account in `USERS_DATABASE_URL` and then exits.

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
the shared schema and then the fixtures from `scripts/test-data.sql`, and uses the Go application's
`run-tests` command to run `go test -count=1 ./...`
without reusing cached test results. It checks that the homepage returns HTTP
status `200` and displays `No place selected`. The test database, its Docker
volume, and its network are removed automatically when the command finishes.
The Go command reads the same configuration as the app and supplies a connection
to `localhost:5433` to the test process using the local PostgreSQL credentials.
The test runner overrides **both** `DATABASE_URL` and `USERS_DATABASE_URL`
with the isolated connection, regardless of the parent environment or `.env`.
It also supplies a test-only connection marker; database tests refuse to run
without the matching isolated configuration. Do not export production URLs
and run database tests directly.
Schema tests also check inserting related photo data in dependency order,
geographic coordinates, cascading deletions, retaining photos without
coordinates, and reapplying the schema without losing data.
Photo API integration tests cover a normal record, an empty result, and a photo
without coordinates, including UTC timestamps, longitude/latitude ordering,
image URLs, place IDs (including `null` without coordinates), public JSON fields, and selection of the content connection for
both photos and locations. A login regression check uses different connection-
local tables to verify that authentication reads usersDB while the header
reads contentDB. A connection check verifies the enforced read-only default
and rejection of content writes.
A pool-wait check verifies that a caller deadline also cancels photo reads while
the content database's only connection is occupied.
Temporary tables exist only on their test connection and are removed when it
closes; production is not used.

Configuration tests set their own example environment values and check missing
settings, `.env` loading, environment precedence, operation without a file, and
feature-specific requirements. They also check that the test runner selects the
isolated database and pgconsole credentials are safely quoted and protected.
Regression tests use fake private
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
are not copied. The shared schema creates no example places, photos, or
platforms. Only `scripts/run-tests.sh` loads the three example places from
`scripts/test-data.sql` into its isolated, disposable test database. Normal
startup does not run that file. Existing local rows are not automatically
deleted by the schema; old unused example places can be removed explicitly
after checking that no coordinates refer to them.

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

The next start creates a fresh database with the current schema but no example
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

pgconsole connects to the local Docker database, not the external content
database used by the homepage. The local photo tables may therefore be empty.
Selecting a table in the sidebar shows its schema. To view local accounts,
click **New Query** and run:

```sql
SELECT id, username, created_at FROM users;
```

The result displays any accounts created locally. To inspect the actual places
and photos used by the homepage, connect a PostgreSQL client to `DATABASE_URL`
with read-only access instead.

## Stop the database

```bash
docker compose down
```
