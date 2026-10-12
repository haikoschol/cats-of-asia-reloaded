# Cats of Asia Reloaded

A small Go web application backed by PostgreSQL with PostGIS. The homepage displays a
randomly selected greeting followed by "anonymous coward" on every request.

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

## User accounts

Create a user account with a username as the only argument:

```bash
./scripts/run.sh create-user peter
```

The command prompts twice for a password without displaying the characters you
type. It creates the account in the local PostgreSQL database and then exits.

Start the web application with `./scripts/run.sh`, then open
[http://localhost:3000/login](http://localhost:3000/login) to sign in. A
successful login redirects to the homepage, which greets you by username. Use
the **Log out** button in the top-right corner to end the session.

## Run tests

Run the automated homepage test with:

```bash
./scripts/run-tests.sh
```

The script starts a separate PostgreSQL test database on port `5433`, seeds the
`greetings` table, and runs `go test -count=1 ./...` without reusing cached test
results. It checks that the homepage returns
HTTP status `200` and contains `anonymous coward`. The test database, its Docker
volume, and its network are removed automatically when the command finishes.

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
