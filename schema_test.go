package main

import (
	"database/sql"
	"os"
	"testing"
)

func TestPhotoSchema(t *testing.T) {
	db, err := sql.Open("pgx", testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	// Insert parent rows before dependents, as a data import must do.
	// Fixtures live only in this transaction and cannot survive the test.
	fixtures := `
		INSERT INTO photos_location (id, city, country, tzoffset)
		VALUES (-1, 'Bangkok', 'Thailand', 420);
		INSERT INTO photos_platform (id, name, profile_url)
		VALUES (-1, 'Example', 'https://example.com/cats');
		INSERT INTO photos_coordinates (id, point, altitude, location_id)
		VALUES (-1, ST_GeogFromText('SRID=4326;POINT(100.5 13.75)'), 10, -1);
		INSERT INTO photos_photo (id, filename, sha256, timestamp, coordinates_id)
		VALUES ('00000000-0000-0000-0000-000000000001', 'cat.jpg', repeat('a', 64), '2026-01-01T12:00:00Z', -1),
		       ('00000000-0000-0000-0000-000000000002', 'no-coordinates.jpg', repeat('b', 64), '2026-01-01T12:00:00Z', NULL);
		INSERT INTO photos_rawmetadata (metadata, photo_id)
		VALUES ('{"camera": "example"}', repeat('a', 64));
		INSERT INTO photos_post (created_at, photo_id, platform_id)
		VALUES ('2026-01-02T12:00:00Z', repeat('a', 64), -1);
	`
	if _, err := tx.Exec(fixtures); err != nil {
		t.Fatal(err)
	}

	var longitude, latitude float64
	if err := tx.QueryRow(`SELECT ST_X(point::geometry), ST_Y(point::geometry)
		FROM photos_coordinates WHERE id = -1`).Scan(&longitude, &latitude); err != nil {
		t.Fatal(err)
	}
	if longitude != 100.5 || latitude != 13.75 {
		t.Fatalf("unexpected longitude/latitude: %v, %v", longitude, latitude)
	}

	// Simulate an existing database from before the demo greeting was removed.
	if _, err := tx.Exec(`
		CREATE TABLE greetings (
			id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			greeting varchar(255) NOT NULL UNIQUE
		);
		INSERT INTO greetings (greeting) VALUES ('hello');
		INSERT INTO users (id, username, hashed_password) OVERRIDING SYSTEM VALUE
		VALUES (-1, 'schema-test-user', repeat('x', 60));
	`); err != nil {
		t.Fatal(err)
	}

	// Reapplying the shared schema removes only the obsolete demo data.
	schema, err := os.ReadFile("scripts/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(string(schema)); err != nil {
		t.Fatalf("reapplying schema: %v", err)
	}
	assertSchemaCount(t, tx, "SELECT count(*) FROM photos_photo", 2)
	assertSchemaCount(t, tx, "SELECT count(*) FROM users WHERE id = -1", 1)
	var greetingsRemoved bool
	if err := tx.QueryRow("SELECT to_regclass('public.greetings') IS NULL").Scan(&greetingsRemoved); err != nil {
		t.Fatal(err)
	}
	if !greetingsRemoved {
		t.Fatal("obsolete greetings table still exists after applying the schema")
	}

	// A location deletion removes coordinates, but retains the photos.
	if _, err := tx.Exec("DELETE FROM photos_location WHERE id = -1"); err != nil {
		t.Fatal(err)
	}
	assertSchemaCount(t, tx, "SELECT count(*) FROM photos_coordinates", 0)
	assertSchemaCount(t, tx, "SELECT count(*) FROM photos_photo WHERE coordinates_id IS NULL", 2)
	assertSchemaCount(t, tx, "SELECT count(*) FROM photos_rawmetadata", 1)
	assertSchemaCount(t, tx, "SELECT count(*) FROM photos_post", 1)

	// A platform deletion removes its posts, but retains photos and metadata.
	if _, err := tx.Exec("DELETE FROM photos_platform WHERE id = -1"); err != nil {
		t.Fatal(err)
	}
	assertSchemaCount(t, tx, "SELECT count(*) FROM photos_post", 0)
	assertSchemaCount(t, tx, "SELECT count(*) FROM photos_photo", 2)
	assertSchemaCount(t, tx, "SELECT count(*) FROM photos_rawmetadata", 1)

	// A photo deletion removes its metadata and publication records.
	if _, err := tx.Exec(`INSERT INTO photos_platform (id, name, profile_url)
		VALUES (-1, 'Example', 'https://example.com/cats');
		INSERT INTO photos_post (created_at, photo_id, platform_id)
		VALUES (now(), repeat('a', 64), -1);
		DELETE FROM photos_photo WHERE sha256 = repeat('a', 64)`); err != nil {
		t.Fatal(err)
	}
	assertSchemaCount(t, tx, "SELECT count(*) FROM photos_rawmetadata", 0)
	assertSchemaCount(t, tx, "SELECT count(*) FROM photos_post", 0)
	assertSchemaCount(t, tx, "SELECT count(*) FROM photos_photo", 1)
}

func assertSchemaCount(t *testing.T, tx *sql.Tx, query string, want int) {
	t.Helper()
	var got int
	if err := tx.QueryRow(query).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s: wanted %d rows, got %d", query, want, got)
	}
}
