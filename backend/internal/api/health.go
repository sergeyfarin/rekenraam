package api

import (
	"io"
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
)

type healthResponse struct {
	Status string `json:"status"`
}

func RegisterRoutes(mux *http.ServeMux, logger *slog.Logger, setupService *app.SetupService) {

	RegisterRoutesWithAuth(mux, logger, setupService, nil, HandlerOptions{})
}

func RegisterRoutesWithAuth(mux *http.ServeMux, logger *slog.Logger, setupService *app.SetupService, authService *app.AuthService, options HandlerOptions) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	mux.HandleFunc("GET /healthz", healthz)
	mux.HandleFunc("GET /api/v1/health", health)
	mux.HandleFunc("GET /api/v1/setup/status", setupStatus(logger, setupService))
	mux.HandleFunc("POST /api/v1/setup/owner", requireSameOrigin(options, createOwner(logger, setupService, options)))
	if authService != nil {
		mux.HandleFunc("GET /api/v1/auth/session", sessionStatus(logger, authService))
		mux.HandleFunc("POST /api/v1/auth/login", requireSameOrigin(options, login(logger, authService, options)))
		mux.HandleFunc("POST /api/v1/auth/logout", logout(logger, authService, options))
	}
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}
