# Development

## Development environment

Run `devenv shell` from the repository root for Go 1.27.4 and Node.js 24.
With only Nix installed, use `nix run github:NixOS/nixpkgs/nixpkgs-unstable#devenv -- shell`.
`devenv.lock` pins the inputs for both tools; update it
deliberately with `devenv update`. `GOTOOLCHAIN=local` prevents Go from downloading
a different toolchain automatically.

`devenv up` starts PostgreSQL 18 with PostGIS, applies the schema to the local
`cats` database, then runs the app. Postgres listens on a project Unix socket
and uses your OS user via peer authentication. Its data persists under
`$DEVENV_STATE/postgres`.

## Configuration

Settings are listed in [.env.example](../.env.example). Environment variables
override `.env`.

- `USERS_DATABASE_URL`: users and sessions. `devenv up` supplies the local socket
  URL. Set it explicitly when running the binary outside this workflow.
- `DATABASE_URL` (optional): overrides where the app reads and writes photos and
  Places. Leave it unset or blank to use `USERS_DATABASE_URL`, including for
  manual upload checks. Use a separate read-only role when manually checking
  against production photo data. Automated tests use disposable PostGIS.
- `HTTP_LISTEN_ADDR`: defaults to `:3000`.
- `SESSION_COOKIE_SECURE`: `false` for local HTTP, `true` behind production HTTPS.
- Image/map settings: public delivery domain/hash and a public Mapbox token.
- Optional services: [uploads](upload.md) and [analytics](analytics.md).

Keep private keys and database URLs out of Git, browser responses and logs.
URL-encode special characters in database credentials.

## Local accounts and database

With `devenv up` running, replace `USERNAME` with your chosen login name:

```sh
devenv shell -- ./scripts/run.sh create-user USERNAME
```

Enter the password twice at the hidden prompts, then sign in at `/login`.
There is no public signup or password-change screen.

Inspect the local database with `devenv shell -- psql -d cats`. Normal startup
adds no sample photos or places. To load photos from an archive, run
`devenv shell -- ./scripts/import-db-dump.sh path/to/backup.dump`.

The old Docker volume `postgres18-data` is retained. Data is not moved
automatically. If you need it, dump the old database and restore it into the
new one. Remove obsolete `POSTGRES_*` settings from your local `.env`.

## Existing photo databases

Before enabling uploads, apply
[001_nullable_altitude.sql](../scripts/migrations/001_nullable_altitude.sql) to the
intended Reloaded database through the deployment procedure. It allows unknown
altitude and prevents duplicate coordinates with unknown height. Grant the
content role write access to the upload tables and their identity sequences.
Do not apply the full development schema to the original application's database.

The schema uses PostGIS geography (longitude first, latitude second). Capture
instants are stored in UTC; unknown altitude is NULL, distinct from zero.

## API reference

| Request | Access | Purpose |
| --- | --- | --- |
| `GET /api/photos` | Public | All photos, ordered by timestamp then UUID |
| `GET /api/photos/nearby` | Public | Photos nearest the supplied GPS point |
| `GET /api/photos/exists?sha256=HASH` | Login | Exact-file duplicate check; returns `exists` as a boolean |
| `GET /api/locations/reverse` | Login | City/country suggestions for a GPS point |
| `POST /api/photos/upload-url` | Login + CSRF | Issue a Cloudflare upload URL |
| `POST /api/photos/save` | Login + CSRF | Validate and save uploaded photo metadata |

Photo results contain `id`, `sha256`, UTC `timestamp`, `location_id`, coordinates,
city/country and `urls` (`desktop`, `mobile`, `smol`). Missing locations are null;
empty results are `[]`. Filenames and raw EXIF are not returned.

Nearby and reverse lookup require `latitude` (-90..90) and `longitude` (-180..180).
Nearby also accepts `radius_km` (>0..100, default 10) and `limit` (1..100, default
10). Invalid, duplicate or unknown parameters return 400. Reverse lookup first
uses stored coordinates; Google requires `GOOGLE_GEOCODING_ENABLED=true` and
`GOOGLE_MAPS_API_KEY`. Multiple matches return `cityCandidates` for selection.

Write requests need the session's `csrf_token` form field or `X-CSRF-Token`
header; JSON uses the header. Anonymous protected API calls return 401; missing
or invalid CSRF returns 403. See [upload details](upload.md#api).

## Tests, builds and deployment

Plain `go test ./...` runs only tests that work in memory. Go test files use
`!integration` for unit tests and `integration` for tests that use files,
loopback HTTP, subprocesses or the test database.

The [test runner](../scripts/run-tests.sh) runs the whole suite by default
(including in CI). For integration tests, it runs the JavaScript checks first,
then starts disposable PostGIS on a private Unix socket and runs the Go suite
with `go test -tags integration ./...`. Both database connections point at
that instance, and the runner removes its temporary data directory.
Provider calls are substituted; automated tests do not upload real images.
[EXIF fixtures](../testdata/upload/README.md) can also be used for manual checks.
Phone file selection and real provider delivery need browser/device checks.

Templates and static assets are embedded: rebuild and restart after changes.
Vendored asset provenance/licenses are retained with
[Leaflet](../static/vendor/leaflet/README.md),
[exifr](../static/vendor/exifr/README.md) and
[Mapbox](../static/vendor/mapbox/README.md).

[GitHub CI](../.github/workflows/release.yml) runs `app:test` on PRs and main.
After successful main tests it runs `app:release`, then publishes
`coa-<commit-prefix>`. To build the Linux amd64 binary and SHA-256 checksum
locally without publishing, run `devenv tasks run app:release` and look in
`dist/`. Release filenames use the checked-out commit; use a clean checkout
to reproduce a published build.
Deployment is separate; use
[cats-of-asia-infra](https://github.com/haikoschol/cats-of-asia-infra).
