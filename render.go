package main

import (
	"bytes"
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
