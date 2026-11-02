package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (app *application) photoExists(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query) != 1 || len(query["sha256"]) != 1 {
		http.Error(w, "exactly one sha256 parameter is required", http.StatusBadRequest)
		return
	}
	hash := query.Get("sha256")
	const invalidHash = "sha256 must contain exactly 64 hexadecimal characters"
	if len(hash) != 64 {
		http.Error(w, invalidHash, http.StatusBadRequest)
		return
	}
	if _, err := hex.DecodeString(hash); err != nil {
		http.Error(w, invalidHash, http.StatusBadRequest)
		return
	}

	// Bound both the photo query and waits for a content connection. Stored
	// hashes use lowercase, as produced by the original browser uploader.
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	var exists bool
	err = app.contentDB.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM photos_photo WHERE sha256 = $1)", strings.ToLower(hash)).Scan(&exists)
	if err != nil {
		log.Printf("photo existence query error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(struct {
		Exists bool `json:"exists"`
	}{Exists: exists}); err != nil {
		log.Printf("photo existence JSON error: %v", err)
	}
}
