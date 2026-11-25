package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"

	"github.com/alexedwards/scs/postgresstore"
	"github.com/alexedwards/scs/v2"
)

type application struct {
	config                 config
	contentDB              *sql.DB
	usersDB                *sql.DB
	sessionManager         *scs.SessionManager
	googleGeocodingClient  *http.Client
	cloudflareImagesClient *http.Client
}

func main() {
	cfg, err := loadConfigFile(".env")
	if err != nil {
		log.Fatal(err)
	}

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "create-user":
			if len(os.Args) != 3 {
				log.Fatal("usage: cats-of-asia-reloaded create-user <username>")
			}
		default:
			log.Fatal("unknown command; use create-user")
		}
	}

	usersDB, err := openUsersDatabase(cfg.UsersDatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer usersDB.Close()

	if len(os.Args) > 1 {
		if err := createUser(usersDB, os.Args[2]); err != nil {
			log.Fatal(err)
		}

		return
	}

	contentDB, err := openContentDatabase(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer contentDB.Close()

	if err := usersDB.Ping(); err != nil {
		log.Fatal(err)
	}

	sessionManager := newSessionManager(cfg)
	sessionManager.Store = postgresstore.New(usersDB)

	app := &application{
		config:         cfg,
		contentDB:      contentDB,
		usersDB:        usersDB,
		sessionManager: sessionManager,
	}

	log.Printf("starting server on %s", cfg.HTTPListenAddr)
	log.Fatal(http.ListenAndServe(cfg.HTTPListenAddr, app.routes()))
}
