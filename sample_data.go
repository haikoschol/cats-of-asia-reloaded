package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
)

type samplePhoto struct {
	ID       string
	Filename string
	Place    string
}

// Fixed identifiers and timestamps keep the example dataset reproducible.
// The place names reference the examples in scripts/schema.sql, not a second
// set of place records. Coordinates and capture dates are fictional.
var samplePhotos = []samplePhoto{
	{ID: "20000000-0000-4000-8000-000000000001", Filename: "schilln.jpeg", Place: "bali"},
	{ID: "20000000-0000-4000-8000-000000000002", Filename: "knuddeln.jpeg", Place: "bali"},
	{ID: "20000000-0000-4000-8000-000000000003", Filename: "kuscheln.jpeg", Place: "vientiane"},
	{ID: "20000000-0000-4000-8000-000000000004", Filename: "noDogorCat.jpeg"},
}

type samplePlace struct {
	City      string
	Country   string
	Longitude float64
	Latitude  float64
	Altitude  float64
}

var samplePlaces = map[string]samplePlace{
	"bali":      {City: "Bali", Country: "Indonesia", Longitude: 115.2, Latitude: -8.7, Altitude: 5},
	"vientiane": {City: "Vientiane", Country: "Laos", Longitude: 102.6, Latitude: 18, Altitude: 10},
}

func validateSampleDatabaseURL(databaseURL string) error {
	// Parse exactly as the PostgreSQL driver does, including host overrides and
	// fallback hosts, rather than checking only the hostname written in the URL.
	cfg, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return fmt.Errorf("seed-sample-data requires a valid local DATABASE_URL")
	}
	local := func(host string) bool {
		if host == "localhost" {
			return true
		}
		ip := net.ParseIP(host)
		return ip != nil && ip.IsLoopback()
	}
	if !local(cfg.Host) {
		return fmt.Errorf("seed-sample-data only supports a local database on localhost or a loopback IP address")
	}
	for _, fallback := range cfg.Fallbacks {
		if !local(fallback.Host) {
			return fmt.Errorf("seed-sample-data only supports local database fallback hosts")
		}
	}
	return nil
}

func seedSampleData(ctx context.Context, db *sql.DB) (int, error) {
	// Read every image before opening the transaction so a missing file cannot
	// leave a partially seeded dataset. Hash the supplied bytes, not dummy values.
	hashes := make([]string, len(samplePhotos))
	for i, photo := range samplePhotos {
		data, err := os.ReadFile(filepath.Join("sample-data", "images", photo.Filename))
		if err != nil {
			return 0, fmt.Errorf("read sample image %s: %w", photo.Filename, err)
		}
		hashes[i] = fmt.Sprintf("%x", sha256.Sum256(data))
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin sample-data transaction: %w", err)
	}
	defer tx.Rollback()

	coordinates := make(map[string]int64)
	inserted := 0
	for i, photo := range samplePhotos {
		// Preserve existing records even if a developer has edited a sample row,
		// or already imported the same image under a different identifier.
		var exists bool
		if err := tx.QueryRowContext(ctx,
			"SELECT EXISTS (SELECT 1 FROM photos_photo WHERE id = $1 OR sha256 = $2)",
			photo.ID, hashes[i],
		).Scan(&exists); err != nil {
			return 0, fmt.Errorf("check sample photo %s: %w", photo.Filename, err)
		}
		if exists {
			continue
		}

		var coordinatesID any
		if photo.Place != "" {
			id, ok := coordinates[photo.Place]
			if !ok {
				id, err = sampleCoordinatesID(ctx, tx, samplePlaces[photo.Place])
				if err != nil {
					return 0, err
				}
				coordinates[photo.Place] = id
			}
			coordinatesID = id
		}

		result, err := tx.ExecContext(ctx, `
			INSERT INTO photos_photo (id, filename, sha256, timestamp, coordinates_id)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT DO NOTHING`,
			photo.ID, photo.Filename, hashes[i],
			time.Date(2026, time.January, 1, 12, i, 0, 0, time.UTC), coordinatesID,
		)
		if err != nil {
			return 0, fmt.Errorf("insert sample photo %s: %w", photo.Filename, err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("count inserted sample photos: %w", err)
		}
		inserted += int(count)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit sample data: %w", err)
	}
	return inserted, nil
}

func sampleCoordinatesID(ctx context.Context, tx *sql.Tx, place samplePlace) (int64, error) {
	var locationID int64
	if err := tx.QueryRowContext(ctx,
		"SELECT id FROM photos_location WHERE city = $1 AND country = $2",
		place.City, place.Country,
	).Scan(&locationID); err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("sample place %s, %s is missing; apply scripts/schema.sql first", place.City, place.Country)
		}
		return 0, fmt.Errorf("find sample place: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO photos_coordinates (location_id, point, altitude)
		VALUES ($1, ST_SetSRID(ST_MakePoint($2, $3), 4326)::geography, $4)
		ON CONFLICT (point, altitude) DO NOTHING`,
		locationID, place.Longitude, place.Latitude, place.Altitude,
	); err != nil {
		return 0, fmt.Errorf("insert sample coordinates: %w", err)
	}

	var coordinatesID, existingLocationID int64
	if err := tx.QueryRowContext(ctx, `
		SELECT id, location_id FROM photos_coordinates
		WHERE point = ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography AND altitude = $3`,
		place.Longitude, place.Latitude, place.Altitude,
	).Scan(&coordinatesID, &existingLocationID); err != nil {
		return 0, fmt.Errorf("find sample coordinates: %w", err)
	}
	if existingLocationID != locationID {
		return 0, fmt.Errorf("sample coordinates for %s, %s already belong to another place; existing data was not changed", place.City, place.Country)
	}
	return coordinatesID, nil
}
