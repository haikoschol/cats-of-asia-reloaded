package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var sampleSchemaNumber atomic.Int64

// Each seed test gets its own schema inside the database supplied by the test
// runner. Committed seed transactions cannot affect the other tests, and all
// fixtures are removed before the isolated database container is shut down.
func sampleTestDatabase(t *testing.T) (*sql.DB, string) {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if err := validateSampleDatabaseURL(databaseURL); err != nil || databaseURL == "" {
		t.Fatal("run database tests with ./scripts/run-tests.sh")
	}
	base, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	schemaName := fmt.Sprintf("sample_seed_test_%d_%d", os.Getpid(), sampleSchemaNumber.Add(1))
	var db *sql.DB
	t.Cleanup(func() {
		if db != nil {
			db.Close()
		}
		if _, err := base.Exec("DROP SCHEMA IF EXISTS " + schemaName + " CASCADE"); err != nil {
			t.Errorf("cleaning up sample test schema: %v", err)
		}
		base.Close()
	})
	if _, err := base.Exec("CREATE SCHEMA " + schemaName); err != nil {
		t.Fatal(err)
	}
	connection, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal("invalid test database URL")
	}
	query := connection.Query()
	query.Set("search_path", schemaName+",public")
	connection.RawQuery = query.Encode()
	testURL := connection.String()
	db, err = sql.Open("pgx", testURL)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile("scripts/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	return db, testURL
}

func sampleCount(t *testing.T, db *sql.DB, query string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("wanted %d rows, got %d for %s", want, got, query)
	}
}

func TestSeedSampleData(t *testing.T) {
	db, _ := sampleTestDatabase(t)
	ctx := context.Background()

	var baliID int64
	if err := db.QueryRow("SELECT id FROM photos_location WHERE city = 'Bali' AND country = 'Indonesia'").Scan(&baliID); err != nil {
		t.Fatal(err)
	}
	// User edits to an existing place must not be reset by the photo seed.
	if _, err := db.Exec("UPDATE photos_location SET tzoffset = 99 WHERE id = $1", baliID); err != nil {
		t.Fatal(err)
	}
	inserted, err := seedSampleData(ctx, db)
	if err != nil || inserted != 4 {
		t.Fatalf("first seed: wanted 4 inserted photos, got %d, error %v", inserted, err)
	}

	var firstCoordinatesID int64
	for i, photo := range samplePhotos {
		var filename, hash string
		var timestamp time.Time
		var coordinatesID sql.NullInt64
		if err := db.QueryRow(`SELECT filename, sha256, timestamp, coordinates_id
			FROM photos_photo WHERE id = $1`, photo.ID).Scan(&filename, &hash, &timestamp, &coordinatesID); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join("sample-data", "images", photo.Filename))
		if err != nil {
			t.Fatal(err)
		}
		wantTime := time.Date(2026, time.January, 1, 12, i, 0, 0, time.UTC)
		if filename != photo.Filename || hash != fmt.Sprintf("%x", sha256.Sum256(data)) || !timestamp.Equal(wantTime) {
			t.Fatalf("unexpected metadata for %s", photo.Filename)
		}
		if photo.Place == "" {
			if coordinatesID.Valid {
				t.Fatal("the fourth photo must not have coordinates")
			}
			continue
		}
		if !coordinatesID.Valid {
			t.Fatalf("missing coordinates for %s", photo.Filename)
		}
		if i == 0 {
			firstCoordinatesID = coordinatesID.Int64
		}
		if i == 1 && coordinatesID.Int64 != firstCoordinatesID {
			t.Fatal("the first two photos must share one coordinate record")
		}
		var city, country string
		var longitude, latitude, altitude float64
		if err := db.QueryRow(`SELECT l.city, l.country, ST_X(c.point::geometry), ST_Y(c.point::geometry), c.altitude
			FROM photos_coordinates c JOIN photos_location l ON l.id = c.location_id
			WHERE c.id = $1`, coordinatesID.Int64).Scan(&city, &country, &longitude, &latitude, &altitude); err != nil {
			t.Fatal(err)
		}
		place := samplePlaces[photo.Place]
		if city != place.City || country != place.Country || longitude != place.Longitude || latitude != place.Latitude || altitude != place.Altitude {
			t.Fatalf("unexpected place or coordinates for %s", photo.Filename)
		}
	}
	inserted, err = seedSampleData(ctx, db)
	if err != nil || inserted != 0 {
		t.Fatalf("repeated seed: wanted 0 inserted photos, got %d, error %v", inserted, err)
	}
	sampleCount(t, db, "SELECT count(*) FROM photos_photo", 4)
	sampleCount(t, db, "SELECT count(*) FROM photos_coordinates", 2)
	sampleCount(t, db, "SELECT count(*) FROM photos_location", 3)
	sampleCount(t, db, "SELECT count(*) FROM photos_location WHERE city = 'Bali' AND tzoffset = 99", 1)

	// Existing sample rows are developer data too; rerunning must retain edits.
	if _, err := db.Exec(`UPDATE photos_photo
		SET filename = 'my-edited-photo.jpeg', timestamp = '2030-01-01T00:00:00Z', coordinates_id = NULL
		WHERE id = $1`, samplePhotos[0].ID); err != nil {
		t.Fatal(err)
	}
	if inserted, err := seedSampleData(ctx, db); err != nil || inserted != 0 {
		t.Fatalf("seed after an edit: got %d, error %v", inserted, err)
	}
	sampleCount(t, db, `SELECT count(*) FROM photos_photo WHERE filename = 'my-edited-photo.jpeg'
		AND timestamp = '2030-01-01T00:00:00Z' AND coordinates_id IS NULL`, 1)
}

func TestSeedPreservesIdentifierAndHashConflicts(t *testing.T) {
	db, _ := sampleTestDatabase(t)
	data, err := os.ReadFile(filepath.Join("sample-data", "images", samplePhotos[1].Filename))
	if err != nil {
		t.Fatal(err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	if _, err := db.Exec(`INSERT INTO photos_photo (id, filename, sha256, timestamp)
		VALUES ($1, 'my-own-photo.jpeg', repeat('a', 64), '2030-01-01T00:00:00Z'),
		('90000000-0000-4000-8000-000000000001', 'already-imported.jpeg', $2, '2031-01-01T00:00:00Z')`,
		samplePhotos[0].ID, hash); err != nil {
		t.Fatal(err)
	}
	inserted, err := seedSampleData(context.Background(), db)
	if err != nil || inserted != 2 {
		t.Fatalf("wanted only the 2 missing photos, got %d, error %v", inserted, err)
	}
	sampleCount(t, db, "SELECT count(*) FROM photos_photo", 4)
	sampleCount(t, db, `SELECT count(*) FROM photos_photo WHERE filename = 'my-own-photo.jpeg'
		AND sha256 = repeat('a', 64) AND timestamp = '2030-01-01T00:00:00Z'`, 1)
	sampleCount(t, db, `SELECT count(*) FROM photos_photo WHERE filename = 'already-imported.jpeg'
		AND timestamp = '2031-01-01T00:00:00Z'`, 1)
	// Both Bali photos were skipped, so no unused Bali coordinates are created.
	sampleCount(t, db, "SELECT count(*) FROM photos_coordinates", 1)
}

func TestSeedReusesExistingCoordinates(t *testing.T) {
	db, _ := sampleTestDatabase(t)
	var existingID int64
	if err := db.QueryRow(`INSERT INTO photos_coordinates (location_id, point, altitude)
		SELECT id, ST_GeogFromText('SRID=4326;POINT(115.2 -8.7)'), 5 FROM photos_location
		WHERE city = 'Bali' AND country = 'Indonesia' RETURNING id`).Scan(&existingID); err != nil {
		t.Fatal(err)
	}
	if inserted, err := seedSampleData(context.Background(), db); err != nil || inserted != 4 {
		t.Fatalf("seed with existing coordinates: got %d, error %v", inserted, err)
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM photos_photo WHERE coordinates_id = $1", existingID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("wanted both Bali photos to reuse existing coordinates, got %d, error %v", count, err)
	}
	sampleCount(t, db, "SELECT count(*) FROM photos_coordinates", 2)
}

func TestSeedRollsBackOnMissingPlace(t *testing.T) {
	db, _ := sampleTestDatabase(t)
	if _, err := db.Exec("DELETE FROM photos_location WHERE city = 'Vientiane' AND country = 'Laos'"); err != nil {
		t.Fatal(err)
	}
	inserted, err := seedSampleData(context.Background(), db)
	if err == nil || !strings.Contains(err.Error(), "Vientiane, Laos is missing") || inserted != 0 {
		t.Fatalf("wanted a missing-place error without partial data, got %d, error %v", inserted, err)
	}
	sampleCount(t, db, "SELECT count(*) FROM photos_photo", 0)
	sampleCount(t, db, "SELECT count(*) FROM photos_coordinates", 0)
	sampleCount(t, db, "SELECT count(*) FROM photos_location", 2)
}

func TestSeedPreservesConflictingCoordinates(t *testing.T) {
	db, _ := sampleTestDatabase(t)
	if _, err := db.Exec(`INSERT INTO photos_coordinates (location_id, point, altitude)
		SELECT id, ST_GeogFromText('SRID=4326;POINT(115.2 -8.7)'), 5 FROM photos_location
		WHERE city = 'Vientiane' AND country = 'Laos'`); err != nil {
		t.Fatal(err)
	}
	inserted, err := seedSampleData(context.Background(), db)
	if err == nil || !strings.Contains(err.Error(), "already belong to another place") || inserted != 0 {
		t.Fatalf("wanted a coordinate conflict without partial data, got %d, error %v", inserted, err)
	}
	sampleCount(t, db, "SELECT count(*) FROM photos_photo", 0)
	sampleCount(t, db, `SELECT count(*) FROM photos_coordinates c JOIN photos_location l ON l.id = c.location_id
		WHERE l.city = 'Vientiane' AND c.altitude = 5`, 1)
}

func TestSeedMissingImage(t *testing.T) {
	t.Chdir(t.TempDir())
	// A nil database is deliberate: missing assets must fail before DB access.
	inserted, err := seedSampleData(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "read sample image schilln.jpeg") || inserted != 0 {
		t.Fatalf("wanted a missing-image error before opening a transaction, got %d, error %v", inserted, err)
	}
}

func TestValidateSampleDatabaseURL(t *testing.T) {
	for _, tt := range []struct {
		name string
		url  string
		ok   bool
	}{
		{name: "localhost", url: "postgres://example:example@localhost:5433/cats?sslmode=disable", ok: true},
		{name: "IPv4", url: "postgres://example:example@127.0.0.1:5433/cats", ok: true},
		{name: "IPv6", url: "postgres://example:example@[::1]:5433/cats", ok: true},
		{name: "remote", url: "postgres://example:example@remote.example/cats"},
		{name: "host override", url: "postgres://example:example@localhost/cats?host=remote.example"},
		{name: "remote fallback", url: "postgres://example:example@localhost,remote.example/cats"},
		{name: "malformed", url: "postgres://example:example@localhost:invalid/cats"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSampleDatabaseURL(tt.url)
			if (err == nil) != tt.ok {
				t.Fatalf("wanted accepted=%t, got error %v", tt.ok, err)
			}
			if err != nil && strings.Contains(err.Error(), tt.url) {
				t.Fatal("validation error exposed the connection URL")
			}
		})
	}
}

func TestSeedSampleDataCommand(t *testing.T) {
	db, databaseURL := sampleTestDatabase(t)
	clearConfigEnvironment(t)
	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("CLOUDFLARE_IMAGES_DOMAIN", "")
	t.Setenv("CLOUDFLARE_IMAGES_ACCOUNT_HASH", "")
	t.Setenv("MAPBOX_ACCESS_TOKEN", "")
	t.Setenv("CATS_SAMPLE_COMMAND_TEST", "1")
	t.Setenv("CATS_SAMPLE_COMMAND_ARGS", "seed-sample-data")
	for _, want := range []string{"Inserted 4 sample photos.", "Inserted 0 sample photos."} {
		command := exec.Command(os.Args[0], "-test.run=^TestSampleDataCommandProcess$")
		output, err := command.CombinedOutput()
		if err != nil || !strings.Contains(string(output), want) {
			t.Fatalf("seed command: wanted %q, got %s, error %v", want, output, err)
		}
	}
	sampleCount(t, db, "SELECT count(*) FROM photos_photo", 4)
	t.Setenv("CATS_SAMPLE_COMMAND_ARGS", "seed-sample-data unexpected-argument")
	command := exec.Command(os.Args[0], "-test.run=^TestSampleDataCommandProcess$")
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "usage: cats-of-asia-reloaded seed-sample-data") {
		t.Fatal("seed command accepted an unexpected positional argument")
	}
}

func TestSampleDataCommandProcess(t *testing.T) {
	if os.Getenv("CATS_SAMPLE_COMMAND_TEST") != "1" {
		t.Skip("only used by the sample-data command test")
	}
	os.Args = append([]string{"cats-of-asia-reloaded"}, strings.Fields(os.Getenv("CATS_SAMPLE_COMMAND_ARGS"))...)
	main()
}
