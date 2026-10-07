package main

import (
	"database/sql"
	"html/template"
	"log"
	"net/http"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type application struct {
	db *sql.DB
}

type templateData struct {
	Greeting string
}

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	var greeting string

	err := app.db.QueryRow(
		"SELECT greeting FROM greetings ORDER BY RANDOM() LIMIT 1",
	).Scan(&greeting)
	if err != nil {
		log.Printf("database query error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	tmpl, err := template.ParseFiles("./template/home.tmpl")
	if err != nil {
		log.Printf("template parse error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := templateData{
		Greeting: greeting,
	}

	err = tmpl.ExecuteTemplate(w, "home.tmpl", data)
	if err != nil {
		log.Printf("template execution error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}

	app := &application{
		db: db,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", app.home)

	log.Println("starting server on http://localhost:3000")
	log.Fatal(http.ListenAndServe(":3000", mux))
}
