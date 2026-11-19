//go:build integration

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
)

func nearbyPhotoID(n int) string {
	return fmt.Sprintf("31000000-0000-4000-8000-%012d", n)
}

func newNearbyTestApp(t *testing.T) *application {
	t.Helper()
	db, err := sql.Open("pgx", testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	// Keep temporary fixtures on one connection, separate from all real tables.
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`
		CREATE TEMP TABLE photos_location (LIKE public.photos_location INCLUDING ALL);
		CREATE TEMP TABLE photos_coordinates (LIKE public.photos_coordinates INCLUDING ALL);
		CREATE TEMP TABLE photos_photo (LIKE public.photos_photo INCLUDING ALL);
		INSERT INTO photos_location (id, city, country, tzoffset)
		VALUES (-101, 'Bangkok', 'Thailand', 420);
		WITH distances(id, metres) AS (
			VALUES (1, 5000), (2, 100), (3, 9999), (4, 10001), (5, 500000), (6, 1000)
		)
		INSERT INTO photos_coordinates (id, location_id, point, altitude)
		SELECT -1000-id, -101,
		       ST_Project(ST_SetSRID(ST_MakePoint(100.5, 13.75), 4326)::geography,
		                  metres::double precision, pi()/2), 0
		FROM distances;
		INSERT INTO photos_photo (id, filename, sha256, timestamp, coordinates_id)
		SELECT ('31000000-0000-4000-8000-' || lpad(i::text, 12, '0'))::uuid,
		       '/private/nearby-cat.jpeg', repeat(md5(i::text), 2),
		       '2026-01-02T10:30:00+07:00', -1000-i
		FROM generate_series(1, 6) i;
		INSERT INTO photos_photo (id, filename, sha256, timestamp)
		VALUES ('31000000-0000-4000-8000-000000000007', '/private/no-coordinates.jpeg',
		        repeat('f', 64), '2026-01-02T10:30:00+07:00');`)
	if err != nil {
		t.Fatal(err)
	}
	return &application{
		contentDB: db, sessionManager: scs.New(),
		config: config{ImageDomain: "images.example.com", ImageAccountHash: "example-account"},
	}
}

func TestNearbyPhotosAPI(t *testing.T) {
	for _, tt := range []struct {
		name, query, extra string
		want               []int
	}{
		{name: "default radius and distance order", query: "latitude=13.75&longitude=100.5", want: []int{2, 6, 1, 3}},
		{name: "custom radius", query: "latitude=13.75&longitude=100.5&radius_km=2", want: []int{2, 6}},
		{name: "just outside default radius included by larger radius", query: "latitude=13.75&longitude=100.5&radius_km=10.002", want: []int{2, 6, 1, 3, 4}},
		{name: "limit", query: "latitude=13.75&longitude=100.5&limit=2", want: []int{2, 6}},
		{name: "maximum radius and limit", query: "latitude=13.75&longitude=100.5&radius_km=100&limit=100", want: []int{2, 6, 1, 3, 4}},
		{name: "zero coordinates and no matches", query: "latitude=0&longitude=0"},
		{name: "coordinate boundaries", query: "latitude=90&longitude=180"},
		{name: "no coordinates", query: "latitude=13.75&longitude=100.5", extra: "DELETE FROM photos_photo WHERE coordinates_id IS NOT NULL;"},
		{name: "same point ordered by UUID", query: "latitude=13.75&longitude=100.5", want: []int{2, 8, 6, 1, 3}, extra: `
			INSERT INTO photos_photo (id, filename, sha256, timestamp, coordinates_id)
			VALUES ('31000000-0000-4000-8000-000000000008', 'same-point.jpeg', repeat('e',64), '2026-01-02T10:30:00+07:00', -1002);`},
		{name: "default limit is ten", query: "latitude=13.75&longitude=100.5", want: []int{2, 6, 10, 11, 12, 13, 14, 15, 16, 17}, extra: `
			INSERT INTO photos_coordinates (id, location_id, point, altitude)
			SELECT -1000-i, -101,
			       ST_Project(ST_SetSRID(ST_MakePoint(100.5, 13.75),4326)::geography,
			                  (2000+i*30)::double precision, pi()/2), 0
			FROM generate_series(10, 21) i;
			INSERT INTO photos_photo (id, filename, sha256, timestamp, coordinates_id)
			SELECT ('31000000-0000-4000-8000-' || lpad(i::text,12,'0'))::uuid,
			       'extra.jpeg', repeat(md5(i::text),2), '2026-01-02T10:30:00+07:00', -1000-i
			FROM generate_series(10, 21) i;`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app := newNearbyTestApp(t)
			if tt.extra != "" {
				if _, err := app.contentDB.Exec(tt.extra); err != nil {
					t.Fatal(err)
				}
			}
			if len(tt.want) == 0 {
				// Empty searches should not depend on image configuration.
				app.config = config{}
			}
			w := httptest.NewRecorder()
			app.routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/photos/nearby?"+tt.query, nil))
			if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("expected public JSON response, got %d: %s", w.Code, w.Body.String())
			}
			var got []photo
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			ids := make([]string, 0, len(got))
			for _, item := range got {
				ids = append(ids, item.ID)
				if item.Latitude == nil || item.Longitude == nil || item.City == nil || *item.City != "Bangkok" {
					t.Fatal("nearby result lost its coordinates or location")
				}
				if !item.Timestamp.Equal(time.Date(2026, 1, 2, 3, 30, 0, 0, time.UTC)) ||
					item.URLs.Desktop != "https://images.example.com/cdn-cgi/imagedelivery/example-account/"+item.ID+"/desktop" {
					t.Fatal("nearby result differs from the shared photo format")
				}
			}
			want := make([]string, 0, len(tt.want))
			for _, n := range tt.want {
				want = append(want, nearbyPhotoID(n))
			}
			if !reflect.DeepEqual(ids, want) {
				t.Fatalf("wanted %v, got %v", want, ids)
			}
			if len(tt.want) == 0 && w.Body.String() != "[]" {
				t.Fatal("empty search did not return []")
			}
			if strings.Contains(w.Body.String(), "/private/") || strings.Contains(w.Body.String(), "filename") {
				t.Fatal("nearby result exposed private photo filenames")
			}
		})
	}
}

func TestNearbyPoolWaitCancellation(t *testing.T) {
	app := newNearbyTestApp(t)
	conn, err := app.contentDB.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// Occupy the only connection: a canceled search must not keep waiting.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/photos/nearby?latitude=13.75&longitude=100.5", nil).WithContext(ctx)
	start := time.Now()
	app.nearbyPhotos(w, r)
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) || w.Code != http.StatusInternalServerError || time.Since(start) > time.Second {
		t.Fatalf("nearby handler did not promptly honor the caller deadline: status %d, context error %v", w.Code, ctx.Err())
	}
}

func TestNearbySpatialIndex(t *testing.T) {
	app := newNearbyTestApp(t)
	tx, err := app.contentDB.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	// This verifies index eligibility. With only six fixture points a sequential
	// scan may legitimately be cheaper; do not force this setting in the app.
	if _, err := tx.Exec("SET LOCAL enable_seqscan = off"); err != nil {
		t.Fatal(err)
	}
	var rawPlan []byte
	err = tx.QueryRow("EXPLAIN (FORMAT JSON) "+nearbyPhotosQuery, 100.5, 13.75, 500, 10).Scan(&rawPlan)
	if err != nil {
		t.Fatal(err)
	}
	type planNode struct {
		IndexCond string     `json:"Index Cond"`
		Plans     []planNode `json:"Plans"`
	}
	var plans []struct{ Plan planNode }
	if err := json.Unmarshal(rawPlan, &plans); err != nil {
		t.Fatal(err)
	}
	var usesPointIndex func(planNode) bool
	usesPointIndex = func(node planNode) bool {
		if strings.Contains(node.IndexCond, "point") && strings.Contains(node.IndexCond, "&&") {
			return true
		}
		for _, child := range node.Plans {
			if usesPointIndex(child) {
				return true
			}
		}
		return false
	}
	for _, plan := range plans {
		if usesPointIndex(plan.Plan) {
			return
		}
	}
	t.Fatalf("nearby search could not use the point index: %s", rawPlan)
}
