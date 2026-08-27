package main

import (
	"log"
	"net/http"
)

// Local dev server. Production runs on Vercel via api/index.go (webhook mode).
func main() {
	fs := http.FileServer(http.Dir("."))

	http.Handle("/safe.glb", fs)
	http.Handle("/safe-open.glb", fs)
	http.Handle("/love_box.glb", fs)
	http.Handle("/sci-fi_box.glb", fs)
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
