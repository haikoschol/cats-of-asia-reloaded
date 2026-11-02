package main

import (
	"log"
	"net/http"
)

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
