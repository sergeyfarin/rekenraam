package api

import (
	"encoding/json"
	"net/http"
)

type healthResponse struct {
	Status string `json:"status"`
}

func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", healthz)
	mux.HandleFunc("GET /api/v1/health", health)
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(healthResponse{Status: "ok"})
}
