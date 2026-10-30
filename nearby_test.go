package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestNearbyInvalidRequests(t *testing.T) {
	// A nil content connection ensures invalid input is rejected before any query.
	app := &application{sessionManager: scs.New()}
	for _, query := range []string{
		"", "latitude=13.75", "longitude=100.5",
		"latitude=&longitude=100.5", "latitude=word&longitude=100.5",
		"latitude=NaN&longitude=100.5", "latitude=Inf&longitude=100.5",
		"latitude=90.01&longitude=100.5", "latitude=-90.01&longitude=100.5",
		"latitude=13.75&longitude=180.01", "latitude=13.75&longitude=-180.01",
		"latitude=13.75&longitude=NaN", "latitude=13.75&longitude=Inf",
		"latitude=13.75&longitude=100.5&radius_km=0",
		"latitude=13.75&longitude=100.5&radius_km=-1",
		"latitude=13.75&longitude=100.5&radius_km=100.01",
		"latitude=13.75&longitude=100.5&radius_km=NaN",
		"latitude=13.75&longitude=100.5&radius_km=Inf",
		"latitude=13.75&longitude=100.5&radius_km=",
		"latitude=13.75&longitude=100.5&limit=0",
		"latitude=13.75&longitude=100.5&limit=-1",
		"latitude=13.75&longitude=100.5&limit=101",
		"latitude=13.75&longitude=100.5&limit=1.5",
		"latitude=13.75&longitude=100.5&limit=NaN",
		"latitude=13.75&longitude=100.5&limit=",
		"latitude=13.75&latitude=0&longitude=100.5",
		"latitude=13.75&longitude=100.5&radius_km=1&radius_km=2",
		"latitude=13.75&longitude=100.5&limit=1&limit=2",
		"latitude=13.75&longitude=100.5&radius=10",
		"latitude=13.75&longitude=100.5&limit=%ZZ",
	} {
		t.Run(query, func(t *testing.T) {
			w := httptest.NewRecorder()
			app.routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/photos/nearby?"+query, nil))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("wanted 400 before database access, got %d", w.Code)
			}
		})
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		w := httptest.NewRecorder()
		app.routes().ServeHTTP(w, httptest.NewRequest(method, "/api/photos/nearby?latitude=0&longitude=0", nil))
		if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "GET" {
			t.Errorf("%s was not rejected with 405 and Allow: GET", method)
		}
	}
}

func TestNearbyParameterBounds(t *testing.T) {
	for _, query := range []string{
		"latitude=0&longitude=0", "latitude=-90&longitude=-180",
		"latitude=90&longitude=180&radius_km=100&limit=100",
		"latitude=0&longitude=0&radius_km=0.001&limit=1",
	} {
		values, err := url.ParseQuery(query)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parseNearbySearch(values); err != nil {
			t.Errorf("valid boundary values were rejected: %s: %v", query, err)
		}
	}
}

func TestNearbyDatabaseError(t *testing.T) {
	app := newNearbyTestApp(t)
	app.contentDB.Close()
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/photos/nearby?latitude=0&longitude=0", nil))
	if w.Code != http.StatusInternalServerError || w.Body.String() != "Internal Server Error\n" {
		t.Fatal("database failure did not produce a generic 500 response")
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
