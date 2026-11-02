package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

// authenticatedUserID returns zero for anonymous or invalidated sessions.
// A stored ID alone is not enough: the user must still exist in usersDB.
func (app *application) authenticatedUserID(ctx context.Context) (int, error) {
	value := app.sessionManager.Get(ctx, "authenticatedUserID")
	if value == nil {
		return 0, nil
	}
	userID, ok := value.(int)
	if !ok || userID <= 0 {
		app.sessionManager.Remove(ctx, "authenticatedUserID")
		return 0, nil
	}

	err := app.usersDB.QueryRowContext(ctx, "SELECT id FROM users WHERE id = $1", userID).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		app.sessionManager.Remove(ctx, "authenticatedUserID")
		return 0, nil
	}
	if err != nil {
		// Fail closed, but retain the session so a temporary database failure
		// does not log the user out.
		return 0, err
	}
	return userID, nil
}

// requireLogin protects browser pages and redirects anonymous visitors to login.
func (app *application) requireLogin(next http.HandlerFunc) http.HandlerFunc {
	return app.requireAuthentication(next, false)
}

// requireLoginJSON protects API actions without returning an HTML login page.
func (app *application) requireLoginJSON(next http.HandlerFunc) http.HandlerFunc {
	return app.requireAuthentication(next, true)
}

func (app *application) requireAuthentication(next http.HandlerFunc, jsonResponse bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		userID, err := app.authenticatedUserID(r.Context())
		if err == nil && userID != 0 {
			next(w, r)
			return
		}

		status := http.StatusUnauthorized
		if err != nil {
			log.Printf("authentication check error: %v", err)
			status = http.StatusInternalServerError
		}
		if jsonResponse {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			if err := json.NewEncoder(w).Encode(map[string]string{"error": http.StatusText(status)}); err != nil {
				log.Printf("authentication JSON error: %v", err)
			}
			return
		}
		if status == http.StatusUnauthorized {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		http.Error(w, http.StatusText(status), status)
	}
}
