package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (app *application) sampleImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	filename := strings.TrimPrefix(r.URL.Path, "/sample-images/")
	allowed := false
	for _, photo := range samplePhotos {
		if filename == photo.Filename {
			allowed = true
			break
		}
	}
	if !allowed {
		http.NotFound(w, r)
		return
	}

	// Do not expose a directory listing or arbitrary repository files. Root also
	// prevents symlinks in the sample directory from accessing files outside it.
	root, err := os.OpenRoot(filepath.Join("sample-data", "images"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer root.Close()
	file, err := root.Open(filename)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, filename, info.ModTime(), file)
}
