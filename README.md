# Cats of Asia Reloaded

A small cat photo website built with Go, PostgreSQL/PostGIS and plain JavaScript.

## Requirements

- [Nix and devenv](https://devenv.sh/getting-started/)

## Local setup

```sh
git clone git@github.com:haikoschol/cats-of-asia-reloaded.git
cd cats-of-asia-reloaded
cp .env.example .env
```

Build without starting services: `devenv tasks run app:build`.

Edit `.env`:

- Leave `DATABASE_URL` unset or blank to use the local database for photos and uploads.
- Set a public `MAPBOX_ACCESS_TOKEN` (`pk.`) and the Cloudflare delivery settings
  to display maps and photos. The example values are placeholders.

```sh
devenv up
```

Open [localhost:3000](http://localhost:3000/). Ctrl+C & `q` stops the app and Postgres.
Restart `devenv up` after changing Go, HTML, CSS or JavaScript; it rebuilds the app.
Keep credentials in the ignored `.env` file.

## Tests

```sh
devenv tasks run app:test
```

Run only one group with `app:test:unit` or `app:test:integration`.

Further guidance: [development](docs/development.md), [uploads](docs/upload.md),
[analytics](docs/analytics.md).
