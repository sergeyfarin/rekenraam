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

	RegisterRoutesWithAuth(mux, logger, setupService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, HandlerOptions{})
}

func RegisterRoutesWithAuth(mux *http.ServeMux, logger *slog.Logger, setupService *app.SetupService, authService *app.AuthService, settingsService *app.SettingsService, bookService *app.BookService, currencyService *app.CurrencyService, institutionService *app.InstitutionService, accountService *app.AccountService, tagService *app.TagService, categoryService *app.CategoryService, payeeService *app.PayeeService, transactionService *app.TransactionService, pricingService *app.PricingService, investmentService *app.InvestmentService, importService *app.ImportService, options HandlerOptions) {
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
	if authService != nil && settingsService != nil {
		mux.HandleFunc("GET /api/v1/settings/preferences", userPreferences(logger, authService, settingsService))
		mux.HandleFunc("PUT /api/v1/settings/preferences", saveUserPreferences(logger, authService, settingsService, options))
	}
	if authService != nil && settingsService != nil && bookService != nil && currencyService != nil && pricingService != nil {
		mux.HandleFunc("GET /api/v1/settings/currencies/page-data", currencySettingsPageData(logger, authService, settingsService, bookService, currencyService, pricingService))
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
		mux.HandleFunc("DELETE /api/v1/institutions/{institution_id}", deleteInstitution(logger, authService, institutionService, options))
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
		mux.HandleFunc("DELETE /api/v1/accounts/{account_id}", deleteAccount(logger, authService, accountService, options))
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
		mux.HandleFunc("GET /api/v1/transactions/deleted", listDeletedTransactions(logger, authService, transactionService))
		mux.HandleFunc("POST /api/v1/transactions", createTransaction(logger, authService, transactionService, options))
		mux.HandleFunc("GET /api/v1/transactions/{transaction_id}", readTransaction(logger, authService, transactionService))
		mux.HandleFunc("PATCH /api/v1/transactions/{transaction_id}", updateTransaction(logger, authService, transactionService, options))
		mux.HandleFunc("POST /api/v1/transactions/{transaction_id}/post", postTransaction(logger, authService, transactionService, options))
		mux.HandleFunc("POST /api/v1/transactions/{transaction_id}/void", voidTransaction(logger, authService, transactionService, options))
		mux.HandleFunc("POST /api/v1/transactions/{transaction_id}/unvoid", unvoidTransaction(logger, authService, transactionService, options))
		mux.HandleFunc("POST /api/v1/transactions/{transaction_id}/soft-delete", softDeleteTransaction(logger, authService, transactionService, options))
		mux.HandleFunc("POST /api/v1/transactions/{transaction_id}/restore", restoreTransaction(logger, authService, transactionService, options))
		mux.HandleFunc("POST /api/v1/transactions/{transaction_id}/correct", correctTransaction(logger, authService, transactionService, options))
		mux.HandleFunc("DELETE /api/v1/transactions/{transaction_id}", deleteDraftTransaction(logger, authService, transactionService, options))
		mux.HandleFunc("POST /api/v1/transactions/{transaction_id}/approve", approveTransaction(logger, authService, transactionService, options))
		mux.HandleFunc("POST /api/v1/transactions/{transaction_id}/move", moveTransaction(logger, authService, transactionService, options))
		mux.HandleFunc("POST /api/v1/accounts/{account_id}/postings/{posting_line_id}/move", movePosting(logger, authService, transactionService, options))
		mux.HandleFunc("POST /api/v1/transactions/reconciliation-impact", createReconciliationImpact(logger, authService, transactionService))
		mux.HandleFunc("POST /api/v1/transactions/{transaction_id}/reconciliation-impact", updateReconciliationImpact(logger, authService, transactionService))
		mux.HandleFunc("POST /api/v1/accounts/{account_id}/reconciliations/start", startReconciliation(logger, authService, transactionService, options))
		mux.HandleFunc("GET /api/v1/reconciliations/{reconciliation_id}", readReconciliation(logger, authService, transactionService))
		mux.HandleFunc("PATCH /api/v1/reconciliations/{reconciliation_id}", updateReconciliationSelection(logger, authService, transactionService, options))
		mux.HandleFunc("POST /api/v1/reconciliations/{reconciliation_id}/preview", previewReconciliation(logger, authService, transactionService, options))
		mux.HandleFunc("POST /api/v1/reconciliations/{reconciliation_id}/finish", finishReconciliation(logger, authService, transactionService, options))
		mux.HandleFunc("POST /api/v1/reconciliations/{reconciliation_id}/void", voidReconciliation(logger, authService, transactionService, options))
		mux.HandleFunc("GET /api/v1/accounts/{account_id}/reconciliation-checkpoints", listReconciliationCheckpoints(logger, authService, transactionService))
		mux.HandleFunc("POST /api/v1/reconciliation-checkpoints/{checkpoint_id}/void", voidReconciliationCheckpoint(logger, authService, transactionService, options))
		mux.HandleFunc("POST /api/v1/accounts/{account_id}/postings/reconciliation-status", changePostingReconciliationStatus(logger, authService, transactionService, options))
	}
	if authService != nil && pricingService != nil {
		mux.HandleFunc("GET /api/v1/pricing/sources", listMarketDataSources(logger, authService, pricingService))
		mux.HandleFunc("GET /api/v1/pricing/prices", listPrices(logger, authService, pricingService))
		mux.HandleFunc("POST /api/v1/pricing/prices", createPrice(logger, authService, pricingService, options))
		mux.HandleFunc("GET /api/v1/pricing/policy", getPricingPolicy(logger, authService, pricingService))
		mux.HandleFunc("PUT /api/v1/pricing/policy", savePricingPolicy(logger, authService, pricingService, options))
		mux.HandleFunc("GET /api/v1/pricing/source-assignments", listPricingSourceAssignments(logger, authService, pricingService))
		mux.HandleFunc("POST /api/v1/pricing/source-assignments", savePricingSourceAssignment(logger, authService, pricingService, options, false))
		mux.HandleFunc("PATCH /api/v1/pricing/source-assignments/{assignment_id}", savePricingSourceAssignment(logger, authService, pricingService, options, true))
		mux.HandleFunc("POST /api/v1/pricing/refresh/run", runPricingRefresh(logger, authService, pricingService, options))
		mux.HandleFunc("GET /api/v1/pricing/refresh/runs", listPricingRefreshRuns(logger, authService, pricingService))
		mux.HandleFunc("GET /api/v1/pricing/source-health", pricingSourceHealth(logger, authService, pricingService))
	}
	if authService != nil && investmentService != nil {
		mux.HandleFunc("GET /api/v1/investments/search", searchInvestments(logger, authService, investmentService))
		mux.HandleFunc("GET /api/v1/investments/instruments", listInvestmentInstruments(logger, authService, investmentService))
		mux.HandleFunc("POST /api/v1/investments/instruments", createInvestmentInstrument(logger, authService, investmentService, options))
		mux.HandleFunc("GET /api/v1/investments/instruments/{instrument_id}", readInvestmentInstrument(logger, authService, investmentService))
		mux.HandleFunc("PATCH /api/v1/investments/instruments/{instrument_id}", updateInvestmentInstrument(logger, authService, investmentService, options))
		mux.HandleFunc("POST /api/v1/investments/holding-accounts", createHoldingAccount(logger, authService, investmentService, options))
		mux.HandleFunc("GET /api/v1/investments/positions", listInvestmentPositions(logger, authService, investmentService))
		mux.HandleFunc("GET /api/v1/investments/lots", listInvestmentLots(logger, authService, investmentService))
		mux.HandleFunc("GET /api/v1/investments/cost-basis-profiles", listCostBasisProfiles(logger, authService, investmentService))
		mux.HandleFunc("POST /api/v1/investments/cost-basis-profiles", saveCostBasisProfile(logger, authService, investmentService, options, false))
		mux.HandleFunc("PATCH /api/v1/investments/cost-basis-profiles/{profile_id}", saveCostBasisProfile(logger, authService, investmentService, options, true))
		mux.HandleFunc("GET /api/v1/investments/dividend-defaults", listDividendDefaults(logger, authService, investmentService))
		mux.HandleFunc("POST /api/v1/investments/dividend-defaults", saveDividendDefault(logger, authService, investmentService, options, false))
		mux.HandleFunc("PATCH /api/v1/investments/dividend-defaults/{default_id}", saveDividendDefault(logger, authService, investmentService, options, true))
		mux.HandleFunc("POST /api/v1/investments/buy", buyInvestment(logger, authService, investmentService, options))
		mux.HandleFunc("POST /api/v1/investments/sell", sellInvestment(logger, authService, investmentService, options))
		mux.HandleFunc("POST /api/v1/investments/sell/preview", sellPreviewInvestment(logger, authService, investmentService))
		mux.HandleFunc("POST /api/v1/investments/dividend", createDividend(logger, authService, investmentService, options))
		mux.HandleFunc("POST /api/v1/investments/reinvested-dividend", createReinvestedDividend(logger, authService, investmentService, options))
		mux.HandleFunc("GET /api/v1/investments/events", listInvestmentEvents(logger, authService, investmentService))
		mux.HandleFunc("GET /api/v1/investments/event-suggestions", listInvestmentEventSuggestions(logger, authService, investmentService))
		mux.HandleFunc("POST /api/v1/investments/event-suggestions/{suggestion_id}/accept", acceptInvestmentEventSuggestion(logger, authService, investmentService, options))
		mux.HandleFunc("POST /api/v1/investments/event-suggestions/{suggestion_id}/ignore", ignoreInvestmentEventSuggestion(logger, authService, investmentService, options))
		mux.HandleFunc("PUT /api/v1/investments/automation-rules", saveInvestmentAutomationRules(logger, authService, investmentService, options))
	}
	if authService != nil && importService != nil {
		mux.HandleFunc("POST /api/v1/imports", startImport(logger, authService, importService, options))
		mux.HandleFunc("GET /api/v1/imports", listImportBatches(logger, authService, importService))
		mux.HandleFunc("GET /api/v1/imports/{batch_id}", getImportBatch(logger, authService, importService))
		mux.HandleFunc("PATCH /api/v1/imports/{batch_id}", patchImportBatch(logger, authService, importService, options))
		mux.HandleFunc("POST /api/v1/imports/{batch_id}/preview-commit", previewCommitImportBatch(logger, authService, importService))
		mux.HandleFunc("POST /api/v1/imports/{batch_id}/commit", commitImportBatch(logger, authService, importService, options))
		mux.HandleFunc("POST /api/v1/imports/{batch_id}/discard", discardImportBatch(logger, authService, importService, options))
	}
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}
