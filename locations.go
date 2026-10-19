package main

import (
	"context"
	"time"
)

type location struct {
	ID      int64
	City    string
	Country string
}

func (app *application) locations(ctx context.Context) ([]location, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	rows, err := app.contentDB.QueryContext(ctx,
		"SELECT id, city, country FROM photos_location ORDER BY country, city, id",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	locations := []location{}
	for rows.Next() {
		var place location
		if err := rows.Scan(&place.ID, &place.City, &place.Country); err != nil {
			return nil, err
		}
		locations = append(locations, place)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return locations, nil
}
