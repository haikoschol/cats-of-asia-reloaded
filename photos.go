package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type photo struct {
	ID         string    `json:"id"`
	SHA256     string    `json:"sha256"`
	Timestamp  time.Time `json:"timestamp"`
	LocationID *int64    `json:"location_id"`
	Latitude   *float64  `json:"latitude"`
	Longitude  *float64  `json:"longitude"`
	City       *string   `json:"city"`
	Country    *string   `json:"country"`
	URLs       imageURLs `json:"urls"`
}

type imageURLs struct {
	Desktop string `json:"desktop"`
	Mobile  string `json:"mobile"`
	Smol    string `json:"smol"`
}

func (cfg config) photoURLs(id string) imageURLs {
	base := fmt.Sprintf("https://%s/cdn-cgi/imagedelivery/%s/%s", cfg.ImageDomain, cfg.ImageAccountHash, id)
	return imageURLs{
		Desktop: base + "/desktop",
		Mobile:  base + "/mobile",
		Smol:    base + "/smol",
	}
}

func (app *application) queryPhotos(ctx context.Context, query string, args ...any) ([]photo, error) {
	// Bound both query time and waits for a shared content connection.
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	tx, err := app.contentDB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	photos := []photo{}
	for rows.Next() {
		var p photo
		if err := rows.Scan(&p.ID, &p.SHA256, &p.Timestamp, &p.LocationID, &p.Latitude, &p.Longitude, &p.City, &p.Country); err != nil {
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

func (app *application) photos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	photos, err := app.queryPhotos(r.Context(), `
		SELECT p.id, p.sha256, p.timestamp,
		       l.id, ST_Y(c.point::geometry), ST_X(c.point::geometry), l.city, l.country
		FROM photos_photo p
		LEFT JOIN photos_coordinates c ON c.id = p.coordinates_id
		LEFT JOIN photos_location l ON l.id = c.location_id
		ORDER BY p.timestamp, p.id`)
	if err != nil {
		log.Printf("photo query error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	writePhotosJSON(w, photos)
}

func writePhotosJSON(w http.ResponseWriter, photos []photo) {
	data, err := json.Marshal(photos)
	if err != nil {
		log.Printf("photo JSON error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
