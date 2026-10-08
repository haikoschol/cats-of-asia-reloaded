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

## Inspecting the local database

`./scripts/run.sh` also starts pgconsole, a browser-based PostgreSQL client.

Open [http://localhost:3001/](http://localhost:3001/) in a browser. No separate
login is required; pgconsole is only available on your local computer.

The `greetings` table is listed in the sidebar. Selecting it shows its schema.
To view the rows, click **New Query** and run:

```sql
SELECT * FROM greetings;
```

The query result displays the seeded greetings. Values can be edited directly
in the result grid and pgconsole will prepare the corresponding SQL update.

## Stop the database

```bash
docker compose down
```
