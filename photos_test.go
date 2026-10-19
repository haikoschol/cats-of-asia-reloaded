package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestPhotosAPI(t *testing.T) {
	const id = "21000000-0000-4000-8000-000000000001"
	const baseURL = "https://images.example.com/cdn-cgi/imagedelivery/example-account-hash/" + id
	latitude, longitude := 13.75, 100.5
	city, country := "Bangkok", "Thailand"
	locationID := int64(-21)
	expected := photo{
		ID: id, SHA256: strings.Repeat("a", 64),
		Timestamp:  time.Date(2026, time.January, 2, 3, 30, 0, 0, time.UTC),
		LocationID: &locationID,
		Latitude:   &latitude, Longitude: &longitude, City: &city, Country: &country,
		URLs: imageURLs{Desktop: baseURL + "/desktop", Mobile: baseURL + "/mobile", Smol: baseURL + "/smol"},
	}
	withoutCoordinates := expected
	withoutCoordinates.LocationID = nil
	withoutCoordinates.Latitude, withoutCoordinates.Longitude = nil, nil
	withoutCoordinates.City, withoutCoordinates.Country = nil, nil

	for _, tt := range []struct {
		name     string
		fixtures string
		want     []photo
	}{
		{name: "empty", want: []photo{}},
		{name: "normal", fixtures: `
			INSERT INTO photos_location (id, city, country, tzoffset) VALUES (-21, 'Bangkok', 'Thailand', 420);
			INSERT INTO photos_coordinates (id, location_id, point, altitude)
			VALUES (-22, -21, ST_GeogFromText('SRID=4326;POINT(100.5 13.75)'), 10);
			INSERT INTO photos_photo (id, filename, sha256, timestamp, coordinates_id)
			VALUES ('21000000-0000-4000-8000-000000000001', '/internal-not-public/cat.jpeg', repeat('a', 64), '2026-01-02T10:30:00+07:00', -22);`,
			want: []photo{expected}},
		{name: "without coordinates", fixtures: `
			INSERT INTO photos_photo (id, filename, sha256, timestamp)
			VALUES ('21000000-0000-4000-8000-000000000001', '/internal-not-public/cat.jpeg', repeat('a', 64), '2026-01-02T10:30:00+07:00');`,
			want: []photo{withoutCoordinates}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			databaseURL := testDatabaseURL(t)
			db, err := sql.Open("pgx", databaseURL)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			// Temporary tables shadow the real tables on this one connection.
			// Closing the connection removes every fixture automatically.
			db.SetMaxOpenConns(1)
			if _, err := db.Exec(`
				CREATE TEMP TABLE photos_location (LIKE public.photos_location INCLUDING ALL);
				CREATE TEMP TABLE photos_coordinates (LIKE public.photos_coordinates INCLUDING ALL);
				CREATE TEMP TABLE photos_photo (LIKE public.photos_photo INCLUDING ALL);
			` + tt.fixtures); err != nil {
				t.Fatal(err)
			}
			cfg := config{DatabaseURL: "postgres://example:fake-private-password@localhost/unused", PostgresPassword: "fake-private-password"}
			if len(tt.want) > 0 {
				cfg.ImageDomain, cfg.ImageAccountHash = "images.example.com", "example-account-hash"
			}
			app := &application{contentDB: db, config: cfg}
			if tt.name == "normal" {
				// The users connection has no content fixture. Locations and photos
				// must both come from the content connection.
				primary, err := sql.Open("pgx", databaseURL)
				if err != nil {
					t.Fatal(err)
				}
				defer primary.Close()
				app.usersDB = primary
				locations, err := app.locations(context.Background())
				if err != nil || len(locations) != 1 || locations[0].ID != locationID || locations[0].City != "Bangkok" {
					t.Fatal("locations did not use the same content connection as photos")
				}
			}
			mux := http.NewServeMux()
			mux.HandleFunc("/api/photos", app.photosAPI)
			server := httptest.NewServer(mux)
			defer server.Close()
			response, err := server.Client().Get(server.URL + "/api/photos")
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("wanted a 200 JSON response, got %d, body %s", response.StatusCode, body)
			}
			var got []photo
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("unexpected photo response: %s", body)
			}
			var fields []map[string]json.RawMessage
			if err := json.Unmarshal(body, &fields); err != nil {
				t.Fatal(err)
			}
			for _, item := range fields {
				if len(item) != 9 {
					t.Fatal("response contains unexpected public fields")
				}
				for _, name := range []string{"id", "sha256", "timestamp", "location_id", "latitude", "longitude", "city", "country", "urls"} {
					if _, exists := item[name]; !exists {
						t.Fatalf("missing JSON field %s", name)
					}
				}
			}
			if strings.Contains(string(body), "internal-not-public") || strings.Contains(string(body), "fake-private-password") {
				t.Fatal("photo response exposed private data")
			}
		})
	}
}

func TestPhotosPoolWaitCancellation(t *testing.T) {
	databaseURL := testDatabaseURL(t)
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	setupCtx, setupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer setupCancel()
	conn, err := db.Conn(setupCtx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Hold the only connection: the photo read must honor a shorter caller
	// deadline while waiting, using the shared content connection.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	app := &application{contentDB: db}
	if _, err := app.photos(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wanted a deadline error while waiting for a connection, got %v", err)
	}
}

func TestContentDatabaseReadOnly(t *testing.T) {
	u, err := url.Parse(testDatabaseURL(t))
	if err != nil || u.Host == "" {
		t.Fatal("run integration tests with ./scripts/run-tests.sh")
	}
	query := u.Query()
	query.Set("default_transaction_read_only", "off")
	u.RawQuery = query.Encode()
	db, err := openContentDatabase(u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var mode string
	if err := db.QueryRow("SHOW default_transaction_read_only").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "on" {
		t.Fatal("photo connection did not enforce read-only mode")
	}
	_, err = db.Exec("INSERT INTO photos_location (city, country, tzoffset) VALUES ('must-not-write', 'test', 0)")
	var databaseError *pgconn.PgError
	if !errors.As(err, &databaseError) || databaseError.Code != "25006" {
		t.Fatal("content connection did not reject a persistent write")
	}
	if _, err := openContentDatabase("postgres://reader:fake-private-password@localhost:invalid/photos"); err == nil || strings.Contains(err.Error(), "fake-private-password") {
		t.Fatal("invalid photo URL did not return a safe configuration error")
	}
}
