package main

import (
	"log"
	"net/http"
)

func main() {
	fs := http.FileServer(http.Dir("."))

	http.Handle("/style.css", fs)
	http.Handle("/app.js", fs)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "index.html")
	})

	log.Println("Dev server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
