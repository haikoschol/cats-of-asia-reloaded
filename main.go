package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/alexedwards/scs/postgresstore"
	"github.com/alexedwards/scs/v2"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type application struct {
	config         config
	db             *sql.DB
	photosDB       *sql.DB
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

	log.Println("starting server on http://localhost:3000")
	log.Fatal(http.ListenAndServe(":3000", app.routes()))
}
