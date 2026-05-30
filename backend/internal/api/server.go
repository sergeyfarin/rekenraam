package api

import (
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
)

func NewHandler(logger *slog.Logger, webHandler http.Handler, setupService *app.SetupService) http.Handler {
	mux := http.NewServeMux()
	RegisterRoutes(mux, logger, setupService)
	mux.HandleFunc("/api/", http.NotFound)
	mux.Handle("/", webHandler)

	return withRequestID(withRequestLogging(logger, withRecovery(logger, mux)))
}
