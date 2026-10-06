package main

// import librarys

import (
	"html/template"
	"log"
	"net/http"
)

// define a handler function which loads the html

func home(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("./ui/html/pages/home.tmpl")
	if err != nil {
		http.Error(w, "Danger internal server error", http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, nil)
	if err != nil {
		http.Error(w, "Danger internal server error", http.StatusInternalServerError)
	}
}

func main() {

// creating servemux & give / the direction to home

	mux := http.NewServeMux()
	mux.HandleFunc("/", home)

// starting the server on port 300 and use our createt mux

	err := http.ListenAndServe(":3000", mux)

// logging error msg if it crashes

	log.Fatal(err)
}
