# Development

## Configuration

Settings are listed in [.env.example](../.env.example). Environment variables
override `.env`; both database URLs are required.

- `DATABASE_URL`: photo data and Places. Uploads need table/sequence write access.
- `USERS_DATABASE_URL`: users and sessions; normally the local Docker database.
- `HTTP_LISTEN_ADDR`: defaults to `:3000`.
- `SESSION_COOKIE_SECURE`: `false` for local HTTP, `true` behind production HTTPS.
- Image/map settings: public delivery domain/hash and a public Mapbox token.
- Optional services: [uploads](upload.md) and [analytics](analytics.md).

Keep private keys and database URLs out of Git, browser responses and logs.
URL-encode special characters in database credentials.

## Local accounts and database

Replace `USERNAME` with your chosen login name:

```sh
./scripts/run.sh create-user USERNAME
```

Enter the password twice at the hidden prompts. The account is created in
`USERS_DATABASE_URL`; restart `./scripts/run.sh` and sign in at `/login`.
There is no public signup or password-change screen.

While the development script runs, [pgconsole](http://localhost:3001/) shows the
local database. External photo data must be inspected through its own connection.
The startup script applies [schema.sql](../scripts/schema.sql) only to the local
Docker database. Normal startup adds no sample photos or places.

Do not delete the development volume `postgres18-data` to troubleshoot a problem.
Older PostgreSQL 17 data is not imported automatically; back up and restore it
before using the new database, and retain the old volume until verified.

For older `.env` files: rename the former local `DATABASE_URL` to
`USERS_DATABASE_URL`, and `PHOTOS_DATABASE_URL` to `DATABASE_URL`.

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

The [test runner](../scripts/run-tests.sh) uses a disposable PostGIS database on
port 5433 for both connections and removes its own test volume afterward.
Provider calls are substituted; automated tests do not upload real images.
[EXIF fixtures](../testdata/upload/README.md) can also be used for manual checks.
Phone file selection and real provider delivery need browser/device checks.

Templates and static assets are embedded: rebuild and restart after changes.
Vendored asset provenance/licenses are retained with
[Leaflet](../static/vendor/leaflet/README.md),
[exifr](../static/vendor/exifr/README.md) and
[Mapbox](../static/vendor/mapbox/README.md).

[GitHub CI](../.github/workflows/release.yml) tests PRs and main. After successful
main tests it publishes a Linux amd64 release named `coa-<commit-prefix>`.
Deployment is separate; use
[cats-of-asia-infra](https://github.com/haikoschol/cats-of-asia-infra).
