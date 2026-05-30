package main

import (
	"context"
	"log"
	"net/http"

	"rekenraam/backend/internal/api"
	"rekenraam/backend/internal/config"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/web"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	database, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	if err := db.Migrate(ctx, database); err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	mux.HandleFunc("/api/", http.NotFound)
	mux.Handle("/", web.Handler())

	log.Printf("rekenraam backend listening on %s", cfg.HTTPAddr)
	log.Fatal(http.ListenAndServe(cfg.HTTPAddr, mux))
}
