package api

import (
	"log/slog"
	"net/http"
	"net/netip"

	"rekenraam/backend/internal/app"
)

type HandlerOptions struct {
	TrustProxyHeaders bool
	TrustedProxyCIDRs []netip.Prefix
}

func NewHandler(logger *slog.Logger, webHandler http.Handler, setupService *app.SetupService, authService *app.AuthService, bookService *app.BookService, options HandlerOptions) http.Handler {
	mux := http.NewServeMux()
	RegisterRoutesWithAuth(mux, logger, setupService, authService, bookService, options)
	mux.HandleFunc("/api/", http.NotFound)
	mux.Handle("/", webHandler)

	return withRequestID(withSecurityHeaders(options, withRequestLogging(logger, withRecovery(logger, mux))))
}
