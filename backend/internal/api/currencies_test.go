package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/app"
)

func TestCurrencyCatalogRequiresAuthentication(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/currencies/catalog", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusUnauthorized, res.Code)
}

func TestCurrencyCatalogReturnsEmbeddedCurrencies(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, _ := createOwnerSession(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/currencies/catalog", nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var body currencyCatalogResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Greater(t, len(body.Currencies), 100)
	assert.Contains(t, catalogCodes(body.Currencies), "USD")
	assert.Contains(t, catalogCodes(body.Currencies), "EUR")
}

func TestCompleteCurrencySetupCreatesDefaultAndAdditionalCurrencies(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/currencies", strings.NewReader(`{
		"default_currency_code": "usd",
		"currencies": [
			{"code": "USD", "name": "US Dollar"},
			{"code": "EUR", "name": "Euro"}
		]
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusCreated, res.Code)

	var body completeCurrencySetupResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Len(t, body.Currencies, 2)
	assert.Equal(t, "USD", body.DefaultCurrency.Code)
	assert.Equal(t, "US Dollar", body.DefaultCurrency.Name)
	assert.Equal(t, 2, body.DefaultCurrency.StandardScale)
	assert.Equal(t, app.InstallStateConfigured, body.Setup.InstallState)
	assert.Equal(t, []string{"owner", "book", "currencies"}, body.Setup.ImplementedSteps)
	assert.Equal(t, "system_accounts", body.Setup.CurrentStep)

	var defaultCurrencyID sql.NullInt64
	err := database.QueryRowContext(context.Background(), `
		SELECT default_currency_commodity_id
		FROM books
		WHERE id = 1
	`).Scan(&defaultCurrencyID)
	require.NoError(t, err)
	require.True(t, defaultCurrencyID.Valid)
	assert.Equal(t, body.DefaultCurrency.ID, defaultCurrencyID.Int64)
}

func TestCompleteCurrencySetupAllowsOmittedAdditionalCurrencies(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/currencies", strings.NewReader(`{
		"default_currency_code": "JPY"
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusCreated, res.Code)

	var body completeCurrencySetupResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Len(t, body.Currencies, 1)
	assert.Equal(t, "JPY", body.DefaultCurrency.Code)
	assert.Equal(t, "Japanese Yen", body.DefaultCurrency.Name)
}

func TestCompleteCurrencySetupRejectsSecondSetup(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")
	completeCurrencySetupForSession(t, handler, sessionCookie, csrfToken, "USD", []setupCurrencySelectionRequest{{Code: "USD", Name: "US Dollar"}})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/currencies", strings.NewReader(`{
		"default_currency_code": "EUR",
		"currencies": [{"code": "EUR", "name": "Euro"}]
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusConflict, res.Code)

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "SETUP_ALREADY_COMPLETE", body.Error.Code)
}

func TestListCurrenciesReturnsCurrentCurrencies(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")
	completeCurrencySetupForSession(t, handler, sessionCookie, csrfToken, "USD", []setupCurrencySelectionRequest{
		{Code: "USD", Name: "US Dollar"},
		{Code: "EUR", Name: "Euro"},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/currencies", nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var body currenciesResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Len(t, body.Currencies, 2)
	assert.Equal(t, []string{"EUR", "USD"}, currencyCodes(body.Currencies))
}

func TestSetDefaultCurrencyChangesPreference(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")
	setup := completeCurrencySetupForSession(t, handler, sessionCookie, csrfToken, "USD", []setupCurrencySelectionRequest{
		{Code: "USD", Name: "US Dollar"},
		{Code: "EUR", Name: "Euro"},
	})

	var euroID int64
	for _, currency := range setup.Currencies {
		if currency.Code == "EUR" {
			euroID = currency.ID
		}
	}
	require.NotZero(t, euroID)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/currencies/"+strconvFormatInt(euroID)+"/default", nil)
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var body setDefaultCurrencyResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "EUR", body.DefaultCurrency.Code)

	var defaultCurrencyID int64
	var updatedByUserID sql.NullInt64
	err := database.QueryRowContext(context.Background(), `
		SELECT default_currency_commodity_id, updated_by_user_id
		FROM books
		WHERE id = 1
	`).Scan(&defaultCurrencyID, &updatedByUserID)
	require.NoError(t, err)
	assert.Equal(t, euroID, defaultCurrencyID)
	require.True(t, updatedByUserID.Valid)
	assert.Equal(t, int64(1), updatedByUserID.Int64)
}

func TestCommodityVersionsAreAppendOnly(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")
	setup := completeCurrencySetupForSession(t, handler, sessionCookie, csrfToken, "USD", []setupCurrencySelectionRequest{{Code: "USD", Name: "US Dollar"}})

	_, err := database.ExecContext(context.Background(), `
		UPDATE commodity_versions
		SET name = 'Changed'
		WHERE commodity_id = ?
	`, setup.DefaultCurrency.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "append-only")

	_, err = database.ExecContext(context.Background(), `
		DELETE FROM commodity_versions
		WHERE commodity_id = ?
	`, setup.DefaultCurrency.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "append-only")
}

func completeCurrencySetupForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, defaultCode string, currencies []setupCurrencySelectionRequest) completeCurrencySetupResponse {
	t.Helper()

	body, err := json.Marshal(completeCurrencySetupRequest{
		DefaultCurrencyCode: defaultCode,
		Currencies:          currencies,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/currencies", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusCreated, res.Code)

	var response completeCurrencySetupResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}

func catalogCodes(currencies []currencyCatalogEntryResponse) []string {
	codes := make([]string, 0, len(currencies))
	for _, currency := range currencies {
		codes = append(codes, currency.Code)
	}
	return codes
}

func currencyCodes(currencies []currencyResponse) []string {
	codes := make([]string, 0, len(currencies))
	for _, currency := range currencies {
		codes = append(codes, currency.Code)
	}
	return codes
}

func strconvFormatInt(value int64) string {
	return strconv.FormatInt(value, 10)
}
