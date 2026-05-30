package main

import (
	"log"
	"net/http"

	"rekenraam/backend/internal/api"
	"rekenraam/backend/internal/config"
	"rekenraam/backend/internal/web"
)

func main() {
	cfg := config.Load()

	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	mux.HandleFunc("/api/", http.NotFound)
	mux.Handle("/", web.Handler())

	log.Printf("rekenraam backend listening on %s", cfg.HTTPAddr)
	log.Fatal(http.ListenAndServe(cfg.HTTPAddr, mux))
}
