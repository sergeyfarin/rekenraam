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

func NewHandler(logger *slog.Logger, webHandler http.Handler, setupService *app.SetupService, authService *app.AuthService, settingsService *app.SettingsService, bookService *app.BookService, currencyService *app.CurrencyService, institutionService *app.InstitutionService, accountService *app.AccountService, tagService *app.TagService, categoryService *app.CategoryService, payeeService *app.PayeeService, transactionService *app.TransactionService, pricingService *app.PricingService, investmentService *app.InvestmentService, options HandlerOptions) http.Handler {
	mux := http.NewServeMux()
	RegisterRoutesWithAuth(mux, logger, setupService, authService, settingsService, bookService, currencyService, institutionService, accountService, tagService, categoryService, payeeService, transactionService, pricingService, investmentService, options)
	mux.HandleFunc("/api/", http.NotFound)
	mux.Handle("/", webHandler)

	return withRequestID(withSecurityHeaders(options, withRequestLogging(logger, withRecovery(logger, mux))))
}
