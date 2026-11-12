package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

func (app *application) createPhotoUploadURL(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Method Not Allowed"})
		return
	}
	result, err := app.requestImageUploadURL(r.Context())
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, errImageUploadConfiguration) {
			status = http.StatusServiceUnavailable
		} else if errors.Is(err, errImageUploadTimeout) {
			status = http.StatusGatewayTimeout
		} else if errors.Is(err, errImageUploadSession) {
			status = http.StatusUnauthorized
		}
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		log.Print("cannot write image upload URL response")
	}
}

func (app *application) upload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	data, err := app.pageData(r)
	if err != nil {
		log.Printf("upload page data error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	data.UploadPage = true
	app.renderPage(w, http.StatusOK, "upload.tmpl", data)
}
