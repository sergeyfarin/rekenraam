package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/app"
)

type forecastAPIFixture struct {
	handler  http.Handler
	database *sql.DB
	session  *http.Cookie
	csrf     string
	currency int64
	checking accountResponse
	expense  accountResponse
}

func newForecastAPIFixture(t *testing.T) forecastAPIFixture {
	t.Helper()
	handler, database := newSetupTestHandler(t)
	session, csrf, currency := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, session, csrf, "Forecast checking", "asset", "checking", currency, 2)
	expense := createLedgerAccount(t, handler, session, csrf, "Forecast expense", "expense", "expense", currency, 2)
	return forecastAPIFixture{handler: handler, database: database, session: session, csrf: csrf, currency: currency, checking: checking, expense: expense}
}

func (f forecastAPIFixture) createFuturePosting(t *testing.T, value string) {
	t.Helper()
	body := `{"transaction_date":"2026-09-08","description":"Forecast item","journal_entries":[{"entry_date":"2026-09-08","postings":[` +
		`{"account_id":` + strconvFormatInt(f.checking.ID) + `,"commodity_id":` + strconvFormatInt(f.currency) + `,"quantity_value":"` + value + `","quantity_scale":2},` +
		`{"account_id":` + strconvFormatInt(f.expense.ID) + `,"commodity_id":` + strconvFormatInt(f.currency) + `,"quantity_value":"-` + value + `","quantity_scale":2}]}]}`
	createTransactionForSession(t, f.handler, f.session, f.csrf, body, http.StatusCreated)
}

func forecastRequest(t *testing.T, handler http.Handler, session *http.Cookie, path string, status int) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if session != nil {
		req.AddCookie(session)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	require.Equalf(t, status, response.Code, "response body: %s", response.Body.String())
	return response
}

func forecastBalancesFor(t *testing.T, fixture forecastAPIFixture, suffix string) forecastBalancesResponse {
	t.Helper()
	response := forecastRequest(t, fixture.handler, fixture.session, "/api/v1/forecasts/balances"+suffix, http.StatusOK)
	var body forecastBalancesResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	return body
}

func TestForecastAPIRequiresOwnerAndIsReadOnly(t *testing.T) {
	fixture := newForecastAPIFixture(t)
	forecastRequest(t, fixture.handler, nil, "/api/v1/forecasts/balances", http.StatusUnauthorized)
	fixture.createFuturePosting(t, "100")
	var beforeTransactions, beforeVersions, beforeAudit int
	require.NoError(t, fixture.database.QueryRow(`SELECT COUNT(*) FROM transactions`).Scan(&beforeTransactions))
	require.NoError(t, fixture.database.QueryRow(`SELECT COUNT(*) FROM transaction_versions`).Scan(&beforeVersions))
	require.NoError(t, fixture.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&beforeAudit))
	balances := forecastBalancesFor(t, fixture, "?horizon_days=2")
	forecastRequest(t, fixture.handler, fixture.session, "/api/v1/forecasts/balance-events?horizon_days=2&date=2026-09-08&basis_token="+balances.BasisToken, http.StatusOK)
	var afterTransactions, afterVersions, afterAudit int
	require.NoError(t, fixture.database.QueryRow(`SELECT COUNT(*) FROM transactions`).Scan(&afterTransactions))
	require.NoError(t, fixture.database.QueryRow(`SELECT COUNT(*) FROM transaction_versions`).Scan(&afterVersions))
	require.NoError(t, fixture.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&afterAudit))
	assert.Equal(t, []int{beforeTransactions, beforeVersions, beforeAudit}, []int{afterTransactions, afterVersions, afterAudit})
}

func TestForecastAPIRejectsAmbiguousQuery(t *testing.T) {
	fixture := newForecastAPIFixture(t)
	paths := []string{
		"?horizon_days=", "?horizon_days=0", "?horizon_days=1.5", "?horizon_days=1&horizon_days=2",
		"?account_id=0", "?account_id=abc", "?include_descendants=1", "?include_descendants=true&include_descendants=false",
		"?as_of=2026-09-07", "?reporting_currency_id=1", "?fx_method=constant_as_of", "?include_drafts=false",
	}
	for _, suffix := range paths {
		t.Run(url.QueryEscape(suffix), func(t *testing.T) {
			forecastRequest(t, fixture.handler, fixture.session, "/api/v1/forecasts/balances"+suffix, http.StatusBadRequest)
		})
	}
	valid := forecastBalancesFor(t, fixture, "")
	assert.Equal(t, 90, valid.HorizonDays)
	assert.True(t, valid.Scope.IncludeDescendants)
}

func TestForecastAPIValidatesPairedFXOptions(t *testing.T) {
	fixture := newForecastAPIFixture(t)
	base := "?horizon_days=1&reporting_currency_id=" + strconvFormatInt(fixture.currency) + "&fx_method=constant_as_of"
	balances := forecastBalancesFor(t, fixture, base)
	require.NotNil(t, balances.Valuation)
	require.NotNil(t, balances.Converted)
	assert.True(t, balances.Valuation.Complete)
	assert.Empty(t, balances.Valuation.UsedRates, "identity conversion must not require an observation")

	eventsPath := "/api/v1/forecasts/balance-events" + base + "&date=2026-09-08&basis_token=" + balances.BasisToken
	forecastRequest(t, fixture.handler, fixture.session, eventsPath, http.StatusOK)
	for _, suffix := range []string{
		"?reporting_currency_id=" + strconvFormatInt(fixture.currency),
		"?fx_method=constant_as_of",
		"?reporting_currency_id=" + strconvFormatInt(fixture.currency) + "&fx_method=latest",
		"?reporting_currency_id=999999&fx_method=constant_as_of",
	} {
		forecastRequest(t, fixture.handler, fixture.session, "/api/v1/forecasts/balances"+suffix, http.StatusBadRequest)
	}
}

func TestForecastMissingFXOmitsWholeConvertedSeries(t *testing.T) {
	fixture := newForecastAPIFixture(t)
	eur, checking := createForeignForecastPosting(t, fixture)
	query := "?horizon_days=2&account_id=" + strconvFormatInt(checking.ID) + "&include_descendants=false&reporting_currency_id=" + strconvFormatInt(fixture.currency) + "&fx_method=constant_as_of"
	balances := forecastBalancesFor(t, fixture, query)
	require.NotNil(t, balances.Valuation)
	assert.False(t, balances.Valuation.Complete)
	require.Len(t, balances.Valuation.Gaps, 1)
	assert.Equal(t, eur.ID, balances.Valuation.Gaps[0].CommodityID)
	assert.Nil(t, balances.Converted)
	require.Len(t, balances.Totals, 1)
	assert.Equal(t, eur.ID, balances.Totals[0].CommodityID, "the exact per-currency series remains available")
}

func TestForecastFXReadDoesNotQueueCoverage(t *testing.T) {
	fixture := newForecastAPIFixture(t)
	eur := createCurrencyForSession(t, fixture.handler, fixture.session, fixture.csrf, `{"code":"EUR","name":"Euro"}`)
	checking := createLedgerAccount(t, fixture.handler, fixture.session, fixture.csrf, "EUR recurring checking", "asset", "checking", eur.ID, 2)
	expense := createLedgerAccount(t, fixture.handler, fixture.session, fixture.csrf, "EUR recurring expense", "expense", "expense", eur.ID, 2)
	generateRecurringDraft(t, fixture.handler, fixture.database, fixture.session, "2026-09-08", checking.ID, expense.ID, eur.ID, 100)
	var before int
	require.NoError(t, fixture.database.QueryRow(`SELECT COUNT(*) FROM background_work_items`).Scan(&before))
	query := "?horizon_days=2&account_id=" + strconvFormatInt(checking.ID) + "&include_descendants=false&reporting_currency_id=" + strconvFormatInt(fixture.currency) + "&fx_method=constant_as_of"
	balances := forecastBalancesFor(t, fixture, query)
	forecastRequest(t, fixture.handler, fixture.session, "/api/v1/forecasts/balance-events"+query+"&date=2026-09-08&basis_token="+balances.BasisToken, http.StatusOK)
	var after int
	require.NoError(t, fixture.database.QueryRow(`SELECT COUNT(*) FROM background_work_items`).Scan(&after))
	assert.Equal(t, before, after)
}

func createForeignForecastPosting(t *testing.T, fixture forecastAPIFixture) (currencyResponse, accountResponse) {
	t.Helper()
	eur := createCurrencyForSession(t, fixture.handler, fixture.session, fixture.csrf, `{"code":"EUR","name":"Euro"}`)
	checking := createLedgerAccount(t, fixture.handler, fixture.session, fixture.csrf, "EUR forecast checking", "asset", "checking", eur.ID, 2)
	expense := createLedgerAccount(t, fixture.handler, fixture.session, fixture.csrf, "EUR forecast expense", "expense", "expense", eur.ID, 2)
	body := `{"transaction_date":"2026-09-08","description":"EUR forecast item","journal_entries":[{"entry_date":"2026-09-08","postings":[` +
		`{"account_id":` + strconvFormatInt(checking.ID) + `,"commodity_id":` + strconvFormatInt(eur.ID) + `,"quantity_value":"100","quantity_scale":2},` +
		`{"account_id":` + strconvFormatInt(expense.ID) + `,"commodity_id":` + strconvFormatInt(eur.ID) + `,"quantity_value":"-100","quantity_scale":2}]}]}`
	createTransactionForSession(t, fixture.handler, fixture.session, fixture.csrf, body, http.StatusCreated)
	return eur, checking
}

func TestForecastAPIReturnsExactWireQuantities(t *testing.T) {
	fixture := newForecastAPIFixture(t)
	fixture.createFuturePosting(t, "9007199254740993")
	response := forecastRequest(t, fixture.handler, fixture.session, "/api/v1/forecasts/balances?horizon_days=2", http.StatusOK)
	assert.Contains(t, response.Body.String(), `"quantity_value":"9007199254740993"`)
	var body forecastBalancesResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	require.NotEmpty(t, body.Series)
	assert.NotNil(t, body.Series[0].Points)
	assert.NotNil(t, body.Diagnostics)

	overflow := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/forecasts/balances", nil)
	writeForecastServiceError(overflow, req, nil, app.LedgerOverflowError{CommodityID: fixture.currency})
	assert.Equal(t, http.StatusUnprocessableEntity, overflow.Code)
	assert.Equal(t, "LEDGER_OVERFLOW", apiErrorCode(t, overflow))
}

func TestForecastAPIDefaultAndEmptyScopes(t *testing.T) {
	handler, _ := newSetupTestHandler(t)
	session, _, _ := setupAccountAPITest(t, handler)
	response := forecastRequest(t, handler, session, "/api/v1/forecasts/balances?horizon_days=1", http.StatusOK)
	var body forecastBalancesResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	assert.Equal(t, "default_cash", body.Scope.Mode)
	assert.Empty(t, body.Scope.ResolvedAccountIDs)
	assert.NotNil(t, body.Scope.ResolvedAccountIDs)
	assert.NotEmpty(t, body.CurrencyOptions)
	assert.NotNil(t, body.Series)
	assert.NotNil(t, body.Totals)

	fixture := newForecastAPIFixture(t)
	selected := forecastBalancesFor(t, fixture, "?account_id="+strconvFormatInt(fixture.checking.ID)+"&include_descendants=false&horizon_days=1")
	assert.Equal(t, "selected_accounts", selected.Scope.Mode)
	assert.Equal(t, []int64{fixture.checking.ID}, selected.Scope.RequestedAccountIDs)
	assert.Equal(t, []int64{fixture.checking.ID}, selected.Scope.ResolvedAccountIDs)
	assert.False(t, selected.Scope.IncludeDescendants)
	assert.NotEmpty(t, selected.Scope.AccountOptions)
}

func TestForecastAPISizeErrorHasNoPartialTotals(t *testing.T) {
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/forecasts/balances", nil)
	writeForecastServiceError(response, req, nil, app.ErrForecastTooLarge)
	assert.Equal(t, http.StatusUnprocessableEntity, response.Code)
	assert.Equal(t, "FORECAST_TOO_LARGE", apiErrorCode(t, response))
	assert.NotContains(t, response.Body.String(), `"totals"`)
}

func TestForecastAPIWiringUsesReadOnlyPool(t *testing.T) {
	fixture := newForecastAPIFixture(t)
	forecastRequest(t, fixture.handler, fixture.session, "/api/v1/forecasts/balances?horizon_days=1", http.StatusOK)
	missing := forecastRequest(t, fixture.handler, fixture.session, "/api/v1/not-a-route", http.StatusNotFound)
	assert.Equal(t, "NOT_FOUND", apiErrorCode(t, missing))
}

func TestForecastAPINoStore(t *testing.T) {
	fixture := newForecastAPIFixture(t)
	balances := forecastRequest(t, fixture.handler, fixture.session, "/api/v1/forecasts/balances?horizon_days=1", http.StatusOK)
	assert.Equal(t, "private, no-store", balances.Header().Get("Cache-Control"))
	errorResponse := forecastRequest(t, fixture.handler, fixture.session, "/api/v1/forecasts/balance-events", http.StatusBadRequest)
	assert.Equal(t, "private, no-store", errorResponse.Header().Get("Cache-Control"))
	assert.NotContains(t, errorResponse.Body.String(), "Forecast checking")
}

func TestForecastEventPagesReconcileToDailyComponents(t *testing.T) {
	fixture := newForecastAPIFixture(t)
	entries := make([]string, 0, 205)
	for index := 0; index < 205; index++ {
		entries = append(entries, `{"entry_date":"2026-09-08","postings":[{"account_id":`+strconvFormatInt(fixture.checking.ID)+`,"commodity_id":`+strconvFormatInt(fixture.currency)+`,"quantity_value":"100","quantity_scale":2},{"account_id":`+strconvFormatInt(fixture.expense.ID)+`,"commodity_id":`+strconvFormatInt(fixture.currency)+`,"quantity_value":"-100","quantity_scale":2}]}`)
	}
	body := `{"transaction_date":"2026-09-08","description":"Paged forecast","journal_entries":[` + strings.Join(entries, ",") + `]}`
	createTransactionForSession(t, fixture.handler, fixture.session, fixture.csrf, body, http.StatusCreated)
	balances := forecastBalancesFor(t, fixture, "?horizon_days=2")
	require.Len(t, balances.Totals, 1)
	assert.Equal(t, "20500", balances.Totals[0].Points[0].PostedDelta.QuantityValue.String())

	basePath := "/api/v1/forecasts/balance-events?horizon_days=2&date=2026-09-08&limit=200&basis_token=" + balances.BasisToken
	firstResponse := forecastRequest(t, fixture.handler, fixture.session, basePath, http.StatusOK)
	var first forecastEventsResponse
	require.NoError(t, json.NewDecoder(firstResponse.Body).Decode(&first))
	assert.Equal(t, 205, first.TotalCount)
	require.Len(t, first.Items, 200)
	require.NotNil(t, first.NextCursor)
	secondResponse := forecastRequest(t, fixture.handler, fixture.session, basePath+"&cursor="+url.QueryEscape(*first.NextCursor), http.StatusOK)
	var second forecastEventsResponse
	require.NoError(t, json.NewDecoder(secondResponse.Body).Decode(&second))
	require.Len(t, second.Items, 5)
	assert.Nil(t, second.NextCursor)
	keys := map[string]bool{}
	for _, event := range append(first.Items, second.Items...) {
		assert.False(t, keys[event.Key])
		keys[event.Key] = true
	}
	assert.Len(t, keys, 205)
}

func TestForecastBasisChangesAfterFinancialMutation(t *testing.T) {
	fixture := newForecastAPIFixture(t)
	fixture.createFuturePosting(t, "100")
	first := forecastBalancesFor(t, fixture, "?horizon_days=2")
	identical := forecastBalancesFor(t, fixture, "?horizon_days=2")
	assert.Equal(t, first.BasisToken, identical.BasisToken)
	fixture.createFuturePosting(t, "200")
	changed := forecastRequest(t, fixture.handler, fixture.session, "/api/v1/forecasts/balance-events?horizon_days=2&date=2026-09-08&basis_token="+first.BasisToken, http.StatusConflict)
	assert.Equal(t, "FORECAST_BASIS_CHANGED", apiErrorCode(t, changed))
	refresh := forecastBalancesFor(t, fixture, "?horizon_days=2")
	assert.NotEqual(t, first.BasisToken, refresh.BasisToken)
}

func TestForecastCursorRejectsChangedRecipeAndMalformedPayload(t *testing.T) {
	fixture := newForecastAPIFixture(t)
	fixture.createFuturePosting(t, "100")
	fixture.createFuturePosting(t, "200")
	balances := forecastBalancesFor(t, fixture, "?horizon_days=2")
	base := "/api/v1/forecasts/balance-events?horizon_days=2&date=2026-09-08&limit=1&basis_token=" + balances.BasisToken
	for _, suffix := range []string{"&cursor=not-base64", "&cursor=" + strings.Repeat("a", app.ForecastMaxCursorBytes+1), "&detail_account_id=999999"} {
		forecastRequest(t, fixture.handler, fixture.session, base+suffix, http.StatusBadRequest)
	}
	firstResponse := forecastRequest(t, fixture.handler, fixture.session, base, http.StatusOK)
	var first forecastEventsResponse
	require.NoError(t, json.NewDecoder(firstResponse.Body).Decode(&first))
	require.NotNil(t, first.NextCursor)
	forecastRequest(t, fixture.handler, fixture.session, base+"&detail_account_id="+strconvFormatInt(fixture.checking.ID)+"&cursor="+url.QueryEscape(*first.NextCursor), http.StatusBadRequest)
	forecastRequest(t, fixture.handler, fixture.session, strings.Replace(base, "horizon_days=2", "horizon_days=1", 1)+"&cursor="+url.QueryEscape(*first.NextCursor), http.StatusConflict)
	forecastRequest(t, fixture.handler, nil, base, http.StatusUnauthorized)
}

func TestForecastReadsLeaveLedgerReportsAndExportsUnchanged(t *testing.T) {
	handler, database := newSetupTestHandler(t)
	f := newExportFixture(t, handler)
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-16",
		posting(f.checking.ID, -1000, 2, f.usdID), posting(f.groceries.ID, 1000, 2, f.usdID)), http.StatusCreated)
	generateRecurringDraft(t, handler, database, f.sessionCookie, "2026-09-08", f.checking.ID, f.groceries.ID, f.usdID, 10000)

	reportPaths := []string{
		"/api/v1/reports/net-worth?start_date=2026-06-01&end_date=2026-09-30&bucket=month",
		"/api/v1/reports/spending?start_date=2026-06-01&end_date=2026-09-30&group_by=category",
		"/api/v1/reports/cashflow?start_date=2026-06-01&end_date=2026-09-30&bucket=month",
	}
	reportsBefore := make([]string, len(reportPaths))
	for i, path := range reportPaths {
		reportsBefore[i] = recurringAPIRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodGet, path, "", http.StatusOK).Body.String()
	}
	csvBefore := downloadLedgerCSV(t, handler, f.sessionCookie, "/api/v1/exports/ledger.csv").body
	qifQuery := "?account_id=" + strconvFormatInt(f.checking.ID) + "&qif_date_layout=mdy"
	qifBefore := downloadQIF(t, handler, f.sessionCookie, qifQuery, http.StatusOK).body
	stateBefore := forecastDomainCounts(t, database)

	query := "?horizon_days=30&account_id=" + strconvFormatInt(f.checking.ID) + "&include_descendants=false"
	balances := forecastBalancesFor(t, forecastAPIFixture{handler: handler, database: database, session: f.sessionCookie}, query)
	forecastRequest(t, handler, f.sessionCookie, "/api/v1/forecasts/balance-events"+query+"&date=2026-09-08&basis_token="+balances.BasisToken, http.StatusOK)

	assert.Equal(t, stateBefore, forecastDomainCounts(t, database), "forecast GETs must not mutate any financial-domain state")
	for i, path := range reportPaths {
		assert.JSONEq(t, reportsBefore[i], recurringAPIRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodGet, path, "", http.StatusOK).Body.String(), path)
	}
	assert.Equal(t, csvBefore, downloadLedgerCSV(t, handler, f.sessionCookie, "/api/v1/exports/ledger.csv").body)
	assert.Equal(t, qifBefore, downloadQIF(t, handler, f.sessionCookie, qifQuery, http.StatusOK).body)
}

func TestForecastDoesNotTouchInvestmentSubledgerOrCheckpoints(t *testing.T) {
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "FXISO")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	buyResponse := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy",
		tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 100000), http.StatusCreated)
	var bought investmentTradeResponse
	require.NoError(t, json.NewDecoder(buyResponse.Body).Decode(&bought))
	holdingPosting := postingByAccount(t, bought.Transaction, holding.ID)
	reconcilePostingForSession(t, handler, f.sessionCookie, f.csrfToken, holding.ID, instrument.CommodityID, holdingPosting, "2026-02-01")
	require.NotEmpty(t, listCheckpointsForSession(t, handler, f.sessionCookie, holding.ID))

	before := forecastDomainCounts(t, database)
	assert.Positive(t, before["investment_lots"], "the no-write assertion must cover existing lot state")
	assert.Positive(t, before["investment_lot_events"], "the no-write assertion must cover existing lot events")
	assert.Positive(t, before["reconciliation_checkpoints"], "the no-write assertion must cover an existing checkpoint")
	query := "?horizon_days=2&account_id=" + strconvFormatInt(f.cashAccount.ID) + "&include_descendants=false"
	fixture := forecastAPIFixture{handler: handler, database: database, session: f.sessionCookie}
	balances := forecastBalancesFor(t, fixture, query)
	forecastRequest(t, handler, f.sessionCookie, "/api/v1/forecasts/balance-events"+query+"&date=2026-09-08&basis_token="+balances.BasisToken, http.StatusOK)
	after := forecastDomainCounts(t, database)
	assert.Equal(t, before, after)
}

func forecastDomainCounts(t *testing.T, database *sql.DB) map[string]int {
	t.Helper()
	tables := []string{
		"transactions", "transaction_versions", "journal_entries", "posting_lines", "posting_versions",
		"recurring_templates", "recurring_occurrences", "audit_events", "background_work_items",
		"reconciliation_checkpoints", "reconciliation_checkpoint_postings",
		"investment_lots", "investment_lot_events",
	}
	counts := make(map[string]int, len(tables))
	for _, table := range tables {
		var count int
		require.NoError(t, database.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&count), table)
		counts[table] = count
	}
	return counts
}
