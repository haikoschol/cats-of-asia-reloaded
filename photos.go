package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

type photo struct {
	ID        string    `json:"id"`
	SHA256    string    `json:"sha256"`
	Timestamp time.Time `json:"timestamp"`
	Latitude  *float64  `json:"latitude"`
	Longitude *float64  `json:"longitude"`
	City      *string   `json:"city"`
	Country   *string   `json:"country"`
	URLs      imageURLs `json:"urls"`
}

type imageURLs struct {
	Desktop string `json:"desktop"`
	Mobile  string `json:"mobile"`
	Smol    string `json:"smol"`
}

func openPhotosDatabase(databaseURL string) (*sql.DB, error) {
	cfg, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		// Driver parse errors may include the original credential-bearing URL.
		return nil, errors.New("cannot parse PHOTOS_DATABASE_URL")
	}
	cfg.RuntimeParams["default_transaction_read_only"] = "on"
	cfg.RuntimeParams["statement_timeout"] = "10000"
	cfg.ConnectTimeout = 5 * time.Second
	db := stdlib.OpenDB(*cfg)
	db.SetMaxOpenConns(3)
	return db, nil
}

func (cfg config) photoURLs(id string) imageURLs {
	base := fmt.Sprintf("https://%s/cdn-cgi/imagedelivery/%s/%s", cfg.ImageDomain, cfg.ImageAccountHash, id)
	return imageURLs{
		Desktop: base + "/desktop",
		Mobile:  base + "/mobile",
		Smol:    base + "/smol",
	}
}

func (app *application) photos(ctx context.Context) ([]photo, error) {
	// Also bound pool waits and reads through the primary database fallback.
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	db := app.photosDB
	if db == nil {
		db = app.db
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `
		SELECT p.id, p.sha256, p.timestamp,
		       ST_Y(c.point::geometry), ST_X(c.point::geometry), l.city, l.country
		FROM photos_photo p
		LEFT JOIN photos_coordinates c ON c.id = p.coordinates_id
		LEFT JOIN photos_location l ON l.id = c.location_id
		ORDER BY p.timestamp, p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	photos := []photo{}
	for rows.Next() {
		var p photo
		if err := rows.Scan(&p.ID, &p.SHA256, &p.Timestamp, &p.Latitude, &p.Longitude, &p.City, &p.Country); err != nil {
			return nil, err
		}
		p.Timestamp = p.Timestamp.UTC()
		photos = append(photos, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Release the connection before doing work that no longer needs the database.
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if len(photos) > 0 {
		if err := app.config.validateImages(); err != nil {
			return nil, err
		}
		for i := range photos {
			photos[i].URLs = app.config.photoURLs(photos[i].ID)
		}
	}
	return photos, nil
}

func (app *application) photosAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	photos, err := app.photos(r.Context())
	if err != nil {
		log.Printf("photo query error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	data, err := json.Marshal(photos)
	if err != nil {
		log.Printf("photo JSON error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
