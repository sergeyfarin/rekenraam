package api

import (
	"encoding/json"
	"net/http"

	"rekenraam/backend/internal/app"
)

func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", healthz)
	mux.HandleFunc("GET /api/hello", hello)
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func hello(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message": app.HelloMessage(),
	})
}
