package main

import (
	"database/sql"
	"log"
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	authenticated := false

	if app.sessionManager.Exists(r.Context(), "authenticatedUserID") {
		userID := app.sessionManager.GetInt(r.Context(), "authenticatedUserID")
		err := app.usersDB.QueryRowContext(r.Context(), "SELECT id FROM users WHERE id = $1", userID).Scan(&userID)
		if err == sql.ErrNoRows {
			app.sessionManager.Remove(r.Context(), "authenticatedUserID")
		} else if err != nil {
			log.Printf("database query error: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		} else {
			authenticated = true
		}
	}

	locations, err := app.locations(r.Context())
	if err != nil {
		log.Printf("location query error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := templateData{
		Authenticated: authenticated,
		Locations:     locations,
	}
	if err := app.config.validateMap(); err != nil {
		data.MapError = "Map unavailable: set MAPBOX_ACCESS_TOKEN to a public Mapbox token (pk.) in .env and restart the app."
	} else {
		data.MapboxAccessToken = app.config.MapboxAccessToken
	}

	app.renderPage(w, http.StatusOK, "home.tmpl", data)
}

func (app *application) logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := app.sessionManager.Destroy(r.Context()); err != nil {
		log.Printf("session destruction error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (app *application) login(w http.ResponseWriter, r *http.Request) {
	if app.sessionManager.Exists(r.Context(), "authenticatedUserID") {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	switch r.Method {
	case http.MethodGet:
		app.renderLogin(w, r, http.StatusOK, templateData{})
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		username := r.PostForm.Get("username")
		password := r.PostForm.Get("password")

		if username == "" || password == "" {
			app.renderLogin(w, r, http.StatusUnprocessableEntity, templateData{
				Username:  username,
				FormError: "Username and password are required.",
			})
			return
		}

		var userID int
		var hashedPassword string
		err := app.usersDB.QueryRow(
			"SELECT id, hashed_password FROM users WHERE username = $1",
			username,
		).Scan(&userID, &hashedPassword)
		if err == sql.ErrNoRows {
			app.renderLogin(w, r, http.StatusUnauthorized, templateData{
				Username:  username,
				FormError: "Username or password is incorrect.",
			})
			return
		}
		if err != nil {
			log.Printf("database query error: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password)) != nil {
			app.renderLogin(w, r, http.StatusUnauthorized, templateData{
				Username:  username,
				FormError: "Username or password is incorrect.",
			})
			return
		}

		if err := app.sessionManager.RenewToken(r.Context()); err != nil {
			log.Printf("session renewal error: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		app.sessionManager.Put(r.Context(), "authenticatedUserID", userID)

		http.Redirect(w, r, "/", http.StatusSeeOther)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (app *application) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", app.home)
	mux.HandleFunc("/login", app.login)
	mux.HandleFunc("/logout", app.logout)
	mux.HandleFunc("/api/photos", app.photosAPI)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
	return app.sessionManager.LoadAndSave(mux)
}
