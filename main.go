package main
import (
	"html/template"
	"log"
	"net/http"
)
func home(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("./template/home.tmpl")
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
	mux := http.NewServeMux()
	mux.HandleFunc("/", home)
	err := http.ListenAndServe(":3000", mux)
	log.Fatal(err)
}
