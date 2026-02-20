package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"planning-poker-go/internal/engine"
	"planning-poker-go/internal/server"
)

func main() {
	pokerEngine := engine.NewEngine()
	hub := server.NewHub()
	go hub.Run()

	srv := &server.Server{
		Engine: pokerEngine,
		Hub:    hub,
	}

	// Cleanup goroutine
	go func() {
		for {
			time.Sleep(10 * time.Minute)
			pokerEngine.CleanupOldRooms(1 * time.Hour)
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/create", srv.HandleCreateRoom)
	mux.HandleFunc("/ws", srv.HandleWS)

	// Serve static files from UI with SPA fallback
	uiPath := "./ui/dist"
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(uiPath, r.URL.Path)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			// Fallback to index.html for SPA routing
			http.ServeFile(w, r, filepath.Join(uiPath, "index.html"))
			return
		}
		http.FileServer(http.Dir(uiPath)).ServeHTTP(w, r)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
