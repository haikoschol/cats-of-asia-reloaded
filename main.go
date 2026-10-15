package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/alexedwards/scs/postgresstore"
	"github.com/alexedwards/scs/v2"
	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

type application struct {
	config         config
	db             *sql.DB
	photosDB       *sql.DB
	sessionManager *scs.SessionManager
}

type location struct {
	ID      int64
	City    string
	Country string
}

type templateData struct {
	Username      string
	Authenticated bool
	FormError     string
	Locations     []location
}

func (app *application) locations(ctx context.Context) ([]location, error) {
	rows, err := app.db.QueryContext(ctx,
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

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	authenticated := false

	if app.sessionManager.Exists(r.Context(), "authenticatedUserID") {
		userID := app.sessionManager.GetInt(r.Context(), "authenticatedUserID")
		err := app.db.QueryRowContext(r.Context(), "SELECT id FROM users WHERE id = $1", userID).Scan(&userID)
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

	tmpl, err := template.ParseFiles("./template/home.tmpl", "./template/header.tmpl")
	if err != nil {
		log.Printf("template parse error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := templateData{
		Authenticated: authenticated,
		Locations:     locations,
	}

	err = tmpl.ExecuteTemplate(w, "home.tmpl", data)
	if err != nil {
		log.Printf("template execution error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
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
		app.renderLogin(w, http.StatusOK, templateData{})
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		username := r.PostForm.Get("username")
		password := r.PostForm.Get("password")

		if username == "" || password == "" {
			app.renderLogin(w, http.StatusUnprocessableEntity, templateData{
				Username:  username,
				FormError: "Username and password are required.",
			})
			return
		}

		var userID int
		var hashedPassword string
		err := app.db.QueryRow(
			"SELECT id, hashed_password FROM users WHERE username = $1",
			username,
		).Scan(&userID, &hashedPassword)
		if err == sql.ErrNoRows {
			app.renderLogin(w, http.StatusUnauthorized, templateData{
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
			app.renderLogin(w, http.StatusUnauthorized, templateData{
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

func (app *application) renderLogin(w http.ResponseWriter, status int, data templateData) {
	tmpl, err := template.ParseFiles("./template/login.tmpl")
	if err != nil {
		log.Printf("template parse error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(status)
	if err := tmpl.ExecuteTemplate(w, "login.tmpl", data); err != nil {
		log.Printf("template execution error: %v", err)
	}
}

func main() {
	cfg, err := loadConfigFile(".env")
	if err != nil {
		log.Fatal(err)
	}

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "prepare-local":
			if len(os.Args) != 2 {
				log.Fatal("usage: cats-of-asia-reloaded prepare-local")
			}
			if err := writePgconsoleConfig(cfg, "pgconsole.toml"); err != nil {
				log.Fatal(err)
			}
			return
		case "run-tests":
			if len(os.Args) != 3 {
				log.Fatal("usage: cats-of-asia-reloaded run-tests <test-port>")
			}
			if err := runTests(cfg, os.Args[2]); err != nil {
				log.Fatal(err)
			}
			return
		case "create-user":
			if len(os.Args) != 3 {
				log.Fatal("usage: cats-of-asia-reloaded create-user <username>")
			}
		default:
			log.Fatal("unknown command; use create-user, prepare-local or run-tests")
		}
	}

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}

	if len(os.Args) > 1 {
		if err := createUser(db, os.Args[2]); err != nil {
			log.Fatal(err)
		}

		return
	}

	var photosDB *sql.DB
	if cfg.PhotosDatabaseURL != "" {
		photosDB, err = openPhotosDatabase(cfg.PhotosDatabaseURL)
		if err != nil {
			log.Fatal(err)
		}
		defer photosDB.Close()
	}

	sessionManager := scs.New()
	sessionManager.Store = postgresstore.New(db)
	sessionManager.Lifetime = 12 * time.Hour

	app := &application{
		config:         cfg,
		db:             db,
		photosDB:       photosDB,
		sessionManager: sessionManager,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", app.home)
	mux.HandleFunc("/login", app.login)
	mux.HandleFunc("/logout", app.logout)
	mux.HandleFunc("/api/photos", app.photosAPI)

	log.Println("starting server on http://localhost:3000")
	log.Fatal(http.ListenAndServe(":3000", sessionManager.LoadAndSave(mux)))
}

func createUser(db *sql.DB, username string) error {
	if username == "" {
		return fmt.Errorf("username must not be empty")
	}

	var exists bool
	err := db.QueryRow("SELECT EXISTS (SELECT 1 FROM users WHERE username = $1)", username).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check whether username exists: %w", err)
	}
	if exists {
		return fmt.Errorf("username %q already exists", username)
	}

	password, err := readPassword("Password: ")
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}
	defer clearPassword(password)

	confirmation, err := readPassword("Confirm password: ")
	if err != nil {
		return fmt.Errorf("read password confirmation: %w", err)
	}
	defer clearPassword(confirmation)

	if len(password) == 0 {
		return fmt.Errorf("password must not be empty")
	}
	if !bytes.Equal(password, confirmation) {
		return fmt.Errorf("passwords do not match")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	_, err = db.Exec(
		"INSERT INTO users (username, hashed_password) VALUES ($1, $2)",
		username,
		hashedPassword,
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	fmt.Printf("Created user %q.\n", username)
	return nil
}

func readPassword(prompt string) ([]byte, error) {
	fmt.Print(prompt)
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	return password, err
}

func clearPassword(password []byte) {
	for i := range password {
		password[i] = 0
	}
}
