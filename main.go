package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/alexedwards/scs/postgresstore"
	"github.com/alexedwards/scs/v2"
)

type application struct {
	config         config
	contentDB      *sql.DB
	usersDB        *sql.DB
	sessionManager *scs.SessionManager
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

	sessionManager := scs.New()
	sessionManager.Store = postgresstore.New(usersDB)
	sessionManager.Lifetime = 12 * time.Hour

	app := &application{
		config:         cfg,
		contentDB:      contentDB,
		usersDB:        usersDB,
		sessionManager: sessionManager,
	}

	log.Println("starting server on http://localhost:3000")
	log.Fatal(http.ListenAndServe(":3000", app.routes()))
}
