# Cats of Asia Reloaded

A small Go web application backed by PostgreSQL. The homepage displays a
randomly selected greeting followed by "anonymous coward" on every request.

## Prerequisites

- Go 1.27 or later
- Docker with Docker Compose

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
values, so the connection settings cannot get out of sync.

Build the application:

```bash
go build .
```

Start the database and the web application:

```bash
./scripts/run.sh
```

Open [http://localhost:3000/](http://localhost:3000/) in a browser. Refresh
the page to receive a different greeting.

Press `Ctrl+C` to stop the web application. The database container continues
to run.

## Stop the database

```bash
docker compose down
```
