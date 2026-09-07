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
