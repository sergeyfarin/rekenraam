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

type Services struct {
	Setup       *app.SetupService
	Auth        *app.AuthService
	Settings    *app.SettingsService
	Book        *app.BookService
	Currency    *app.CurrencyService
	Institution *app.InstitutionService
	Account     *app.AccountService
	Tag         *app.TagService
	Category    *app.CategoryService
	Payee       *app.PayeeService
	Transaction *app.TransactionService
	Pricing     *app.PricingService
	Investment  *app.InvestmentService
	Import      *app.ImportService
}

func NewHandler(logger *slog.Logger, webHandler http.Handler, services Services, options HandlerOptions) http.Handler {
	mux := http.NewServeMux()
	RegisterRoutesWithAuth(mux, logger, services, options)
	mux.HandleFunc("/api/", http.NotFound)
	mux.Handle("/", webHandler)

	return withRequestID(withSecurityHeaders(options, withRequestLogging(logger, withRecovery(logger, mux))))
}
