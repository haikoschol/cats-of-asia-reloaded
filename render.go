package main

import (
	"bytes"
	"database/sql"
	"html/template"
	"log"
	"net/http"
)

type templateData struct {
	Username          string
	Authenticated     bool
	FormError         string
	Locations         []location
	MapboxAccessToken string
	MapError          string
	FavoritesPage     bool
}

func (app *application) pageData(r *http.Request) (templateData, error) {
	data := templateData{}
	if app.sessionManager.Exists(r.Context(), "authenticatedUserID") {
		userID := app.sessionManager.GetInt(r.Context(), "authenticatedUserID")
		err := app.usersDB.QueryRowContext(r.Context(), "SELECT id FROM users WHERE id = $1", userID).Scan(&userID)
		if err == sql.ErrNoRows {
			app.sessionManager.Remove(r.Context(), "authenticatedUserID")
		} else if err != nil {
			return data, err
		} else {
			data.Authenticated = true
		}
	}
	locations, err := app.locations(r.Context())
	data.Locations = locations
	return data, err
}

func (app *application) renderLogin(w http.ResponseWriter, r *http.Request, status int, data templateData) {
	locations, err := app.locations(r.Context())
	if err != nil {
		log.Printf("location query error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	data.Locations = locations
	app.renderPage(w, status, "login.tmpl", data)
}

func (app *application) renderPage(w http.ResponseWriter, status int, page string, data templateData) {
	// Parse one page at a time so its blocks cannot overwrite another page's.
	tmpl, err := template.ParseFS(templateFiles, "template/base.tmpl", "template/header.tmpl", "template/"+page)
	if err != nil {
		log.Printf("template parse error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "base", data); err != nil {
		log.Printf("template execution error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := buf.WriteTo(w); err != nil {
		log.Printf("template response error: %v", err)
	}
}
