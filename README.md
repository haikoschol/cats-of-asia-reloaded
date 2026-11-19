# Cats of Asia Reloaded

A small cat photo website built with Go, PostgreSQL/PostGIS and plain JavaScript.

## Requirements

- [Nix and devenv](https://devenv.sh/getting-started/) for pinned Go and Node.js
- Docker with Compose v2 supporting `up --wait --wait-timeout`
- AMD64 container support, including emulation on Apple Silicon

## Local setup

```sh
git clone git@github.com:haikoschol/cats-of-asia-reloaded.git
cd cats-of-asia-reloaded
devenv shell
cp .env.example .env
```

Build without starting services: `devenv tasks run app:build`.

Edit `.env`:

- Set `POSTGRES_USER`, `POSTGRES_PASSWORD` and `POSTGRES_DB` for the local database.
- Set `USERS_DATABASE_URL` to match those credentials on `localhost:5432`.
- Leave `DATABASE_URL` unset or blank to use the local database for photos and uploads.
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
devenv tasks run app:test
```

Run only one group with `app:test:unit` or `app:test:integration`.

Further guidance: [development](docs/development.md), [uploads](docs/upload.md),
[analytics](docs/analytics.md).
