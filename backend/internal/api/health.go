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

	RegisterRoutesWithAuth(mux, logger, setupService, nil, nil, nil, nil, nil, nil, nil, nil, nil, HandlerOptions{})
}

func RegisterRoutesWithAuth(mux *http.ServeMux, logger *slog.Logger, setupService *app.SetupService, authService *app.AuthService, bookService *app.BookService, currencyService *app.CurrencyService, institutionService *app.InstitutionService, accountService *app.AccountService, tagService *app.TagService, categoryService *app.CategoryService, payeeService *app.PayeeService, transactionService *app.TransactionService, options HandlerOptions) {
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
	if authService != nil && bookService != nil {
		mux.HandleFunc("GET /api/v1/books/current", currentBook(logger, authService, bookService))
		mux.HandleFunc("POST /api/v1/setup/book", createBook(logger, authService, bookService, options))
	}
	if authService != nil && currencyService != nil {
		mux.HandleFunc("GET /api/v1/currencies/catalog", currencyCatalog(logger, authService, currencyService))
		mux.HandleFunc("GET /api/v1/currencies", listCurrencies(logger, authService, currencyService))
		mux.HandleFunc("POST /api/v1/currencies", createCurrency(logger, authService, currencyService, options))
		mux.HandleFunc("POST /api/v1/currencies/{commodity_id}/default", setDefaultCurrency(logger, authService, currencyService, options))
		mux.HandleFunc("POST /api/v1/setup/currencies", completeCurrencySetup(logger, authService, currencyService, options))
	}
	if authService != nil && institutionService != nil {
		mux.HandleFunc("GET /api/v1/institutions", listInstitutions(logger, authService, institutionService))
		mux.HandleFunc("POST /api/v1/institutions", createInstitution(logger, authService, institutionService, options))
		mux.HandleFunc("GET /api/v1/institutions/{institution_id}", readInstitution(logger, authService, institutionService))
		mux.HandleFunc("PATCH /api/v1/institutions/{institution_id}", updateInstitution(logger, authService, institutionService, options))
		mux.HandleFunc("POST /api/v1/institutions/{institution_id}/archive", archiveInstitution(logger, authService, institutionService, options))
		mux.HandleFunc("POST /api/v1/institutions/{institution_id}/restore", restoreInstitution(logger, authService, institutionService, options))
		mux.HandleFunc("GET /api/v1/institutions/{institution_id}/versions", listInstitutionVersions(logger, authService, institutionService))
	}
	if authService != nil && accountService != nil {
		mux.HandleFunc("GET /api/v1/accounts", listAccounts(logger, authService, accountService))
		mux.HandleFunc("POST /api/v1/accounts", createAccount(logger, authService, accountService, options))
		mux.HandleFunc("GET /api/v1/accounts/{account_id}", readAccount(logger, authService, accountService))
		if transactionService != nil {
			mux.HandleFunc("GET /api/v1/accounts/{account_id}/register", accountRegister(logger, authService, transactionService))
		}
		mux.HandleFunc("PATCH /api/v1/accounts/{account_id}", updateAccount(logger, authService, accountService, options))
		mux.HandleFunc("POST /api/v1/accounts/{account_id}/close", closeAccount(logger, authService, accountService, options))
		mux.HandleFunc("POST /api/v1/accounts/{account_id}/reopen", reopenAccount(logger, authService, accountService, options))
		mux.HandleFunc("POST /api/v1/accounts/{account_id}/archive", archiveAccount(logger, authService, accountService, options))
		mux.HandleFunc("POST /api/v1/accounts/{account_id}/restore", restoreAccount(logger, authService, accountService, options))
		mux.HandleFunc("GET /api/v1/accounts/{account_id}/versions", listAccountVersions(logger, authService, accountService))
		mux.HandleFunc("POST /api/v1/setup/system-accounts", completeSystemAccountsSetup(logger, authService, accountService, options))
	}
	if authService != nil && tagService != nil {
		mux.HandleFunc("GET /api/v1/tags", listTags(logger, authService, tagService))
		mux.HandleFunc("POST /api/v1/tags", createTag(logger, authService, tagService, options))
		mux.HandleFunc("GET /api/v1/tags/{tag_id}", readTag(logger, authService, tagService))
		mux.HandleFunc("PATCH /api/v1/tags/{tag_id}", updateTag(logger, authService, tagService, options))
		mux.HandleFunc("POST /api/v1/tags/{tag_id}/archive", archiveTag(logger, authService, tagService, options))
		mux.HandleFunc("POST /api/v1/tags/{tag_id}/restore", restoreTag(logger, authService, tagService, options))
	}
	if authService != nil && categoryService != nil {
		mux.HandleFunc("GET /api/v1/categories", listCategories(logger, authService, categoryService))
		mux.HandleFunc("POST /api/v1/categories", createCategory(logger, authService, categoryService, options))
		mux.HandleFunc("GET /api/v1/categories/{category_id}", readCategory(logger, authService, categoryService))
		mux.HandleFunc("PATCH /api/v1/categories/{category_id}", updateCategory(logger, authService, categoryService, options))
		mux.HandleFunc("POST /api/v1/categories/{category_id}/disable", disableCategory(logger, authService, categoryService, options))
		mux.HandleFunc("POST /api/v1/categories/{category_id}/restore", restoreCategory(logger, authService, categoryService, options))
		mux.HandleFunc("DELETE /api/v1/categories/{category_id}", deleteCategory(logger, authService, categoryService, options))
		mux.HandleFunc("POST /api/v1/setup/categories", completeCategoriesSetup(logger, authService, categoryService, options))
	}
	if authService != nil && payeeService != nil {
		mux.HandleFunc("GET /api/v1/payees", listPayees(logger, authService, payeeService))
		mux.HandleFunc("POST /api/v1/payees", createPayee(logger, authService, payeeService, options))
		mux.HandleFunc("GET /api/v1/payees/{payee_id}", readPayee(logger, authService, payeeService))
		mux.HandleFunc("PATCH /api/v1/payees/{payee_id}", updatePayee(logger, authService, payeeService, options))
		mux.HandleFunc("POST /api/v1/payees/{payee_id}/archive", archivePayee(logger, authService, payeeService, options))
		mux.HandleFunc("POST /api/v1/payees/{payee_id}/restore", restorePayee(logger, authService, payeeService, options))
	}
	if authService != nil && transactionService != nil {
		mux.HandleFunc("GET /api/v1/ledger/account-balances", accountBalances(logger, authService, transactionService))
		mux.HandleFunc("GET /api/v1/ledger/category-totals", categoryTotals(logger, authService, transactionService))
		mux.HandleFunc("GET /api/v1/ledger/net-worth", netWorth(logger, authService, transactionService))
		mux.HandleFunc("GET /api/v1/transactions", listTransactions(logger, authService, transactionService))
		mux.HandleFunc("POST /api/v1/transactions", createTransaction(logger, authService, transactionService, options))
		mux.HandleFunc("GET /api/v1/transactions/{transaction_id}", readTransaction(logger, authService, transactionService))
		mux.HandleFunc("PATCH /api/v1/transactions/{transaction_id}", updateTransaction(logger, authService, transactionService, options))
		mux.HandleFunc("POST /api/v1/transactions/{transaction_id}/post", postTransaction(logger, authService, transactionService, options))
		mux.HandleFunc("POST /api/v1/transactions/{transaction_id}/void", voidTransaction(logger, authService, transactionService, options))
		mux.HandleFunc("POST /api/v1/transactions/{transaction_id}/correct", correctTransaction(logger, authService, transactionService, options))
		mux.HandleFunc("DELETE /api/v1/transactions/{transaction_id}", deleteDraftTransaction(logger, authService, transactionService, options))
	}
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}
