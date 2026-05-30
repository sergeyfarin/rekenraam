package api

import (
	"log/slog"
	"net/http"
)

func NewHandler(logger *slog.Logger, webHandler http.Handler) http.Handler {
	mux := http.NewServeMux()
	RegisterRoutes(mux)
	mux.HandleFunc("/api/", http.NotFound)
	mux.Handle("/", webHandler)

	return withRequestID(withRequestLogging(logger, withRecovery(logger, mux)))
}
