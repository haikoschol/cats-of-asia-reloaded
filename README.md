# Cats of Asia Reloaded

A small cat photo website built with Go, PostgreSQL/PostGIS and plain JavaScript.

## Requirements

- Go 1.27+, Node.js 22+ (tests only; no npm install)
- Docker with Compose v2 supporting `up --wait --wait-timeout`
- AMD64 container support, including emulation on Apple Silicon

## Local setup

```sh
git clone git@github.com:haikoschol/cats-of-asia-reloaded.git
cd cats-of-asia-reloaded
cp .env.example .env
```

Edit `.env`:

- Set `POSTGRES_USER`, `POSTGRES_PASSWORD` and `POSTGRES_DB` for the local database.
- Set `USERS_DATABASE_URL` to match those credentials on `localhost:5432`.
- Obtain `DATABASE_URL` for the photo database from its owner, or point it at the
  local database too. Uploads require write permissions.
- Set a public `MAPBOX_ACCESS_TOKEN` (`pk.`) and the Cloudflare delivery settings
  to display maps and photos. The example values are placeholders.

```sh
./scripts/run.sh
```

Open [localhost:3000](http://localhost:3000/). Ctrl+C stops the app and containers.
Restart the script after changing Go, HTML, CSS or JavaScript; it rebuilds the app.
Keep credentials in the ignored `.env` file.

## Tests

```sh
./scripts/run-tests.sh
```

Further guidance: [development](docs/development.md), [uploads](docs/upload.md),
[analytics](docs/analytics.md).
