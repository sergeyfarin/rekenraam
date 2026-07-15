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

	"rekenraam/backend/internal/exact"
)

// --- Bootstrap ---

type investmentAPITestFixture struct {
	sessionCookie *http.Cookie
	csrfToken     string
	commodityID   int64
	cashAccount   accountResponse
	incomeAccount accountResponse
}

func bootstrapInvestmentAPITest(t *testing.T, handler http.Handler) investmentAPITestFixture {
	t.Helper()
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)

	// Buy/Sell require the commodity_trading system account, created by the
	// setup wizard's system-accounts step (see TestSystemAccountSetupCreatesRolesAndProtectsThem).
	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/system-accounts", nil)
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, http.StatusCreated, res.Code, res.Body.String())

	cashAccount := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Checking", "asset", "checking", commodityID, 2)
	incomeAccount := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Dividend Income", "income", "income", commodityID, 2)
	return investmentAPITestFixture{
		sessionCookie: sessionCookie, csrfToken: csrfToken, commodityID: commodityID,
		cashAccount: cashAccount, incomeAccount: incomeAccount,
	}
}

// --- Request helpers ---

func doInvestmentRequest(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, method string, path string, body any, wantStatus int) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		reader = strings.NewReader(string(encoded))
	} else {
		reader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if csrfToken != "" {
		req.Header.Set(csrfTokenHeader, csrfToken)
		setSameOrigin(req)
	}
	if sessionCookie != nil {
		req.AddCookie(sessionCookie)
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, wantStatus, res.Code, res.Body.String())
	return res
}

func createInstrumentForSession(t *testing.T, handler http.Handler, f investmentAPITestFixture, symbol string) investmentInstrumentResponse {
	t.Helper()
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/instruments", investmentInstrumentRequest{
		CommodityCode: symbol, InstrumentType: "etf", DisplayName: "Test Instrument " + symbol, Symbol: symbol,
		QuantityScale: 6, PriceScale: 4, EffectiveFrom: "2026-01-01",
		Identifiers: json.RawMessage(`{}`), Metadata: json.RawMessage(`{}`),
	}, http.StatusCreated)
	var response investmentInstrumentResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}

func createHoldingAccountForSession(t *testing.T, handler http.Handler, f investmentAPITestFixture, instrumentID int64) accountResponse {
	t.Helper()
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/holding-accounts", holdingAccountRequest{
		InstrumentID: instrumentID, Name: "Holding", OpenedOn: "2026-01-01", EffectiveFrom: "2026-01-01",
	}, http.StatusCreated)
	var response accountResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}

func tradeRequestBody(f investmentAPITestFixture, holdingAccountID, commodityID int64, quantity string, cashValue int64) investmentTradeRequest {
	qty, err := exact.Parse(quantity)
	if err != nil {
		panic(err)
	}
	return investmentTradeRequest{
		TransactionDate: "2026-02-01", CommodityID: commodityID, HoldingAccountID: holdingAccountID,
		CashAccountID: f.cashAccount.ID, QuantityValue: qty, QuantityScale: 0,
		CashAmountValue: cashValue, CashAmountScale: 2, CashCommodityID: f.commodityID,
	}
}

// --- Lifecycle ---

func TestInvestmentLifecycle_InstrumentBuyPreviewSellDividendReflectEverywhere(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	instrument := createInstrumentForSession(t, handler, f, "VWRL")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)

	buyRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy",
		tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 100000), http.StatusCreated)
	var bought investmentTradeResponse
	require.NoError(t, json.NewDecoder(buyRes.Body).Decode(&bought))
	require.NotNil(t, bought.LotID)

	previewBody := investmentTradeRequest{
		TransactionDate: "2026-03-01", CommodityID: instrument.CommodityID, HoldingAccountID: holding.ID,
		CashAccountID: f.cashAccount.ID, QuantityValue: exact.New(10), QuantityScale: 0,
		CashAmountValue: 120000, CashAmountScale: 2, CashCommodityID: f.commodityID,
	}
	previewRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, "/api/v1/investments/sell/preview", previewBody, http.StatusOK)
	var preview sellPreviewResponse
	require.NoError(t, json.NewDecoder(previewRes.Body).Decode(&preview))
	assert.Equal(t, int64(20000), preview.RealizedGain)

	sellRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/sell", previewBody, http.StatusCreated)
	var sold investmentTradeResponse
	require.NoError(t, json.NewDecoder(sellRes.Body).Decode(&sold))
	require.Len(t, sold.Allocations, 1)
	assert.Equal(t, preview.Allocations[0].CostBasisValue, sold.Allocations[0].CostBasisValue, "preview and commit must agree")

	dividendRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/dividend", dividendRequest{
		TransactionDate: "2026-04-01", CashAccountID: f.cashAccount.ID, CashCommodityID: f.commodityID,
		IncomeAccountID: &f.incomeAccount.ID, AmountValue: 5000, AmountScale: 2,
	}, http.StatusCreated)
	_ = dividendRes

	lotsRes := httptest.NewRequest(http.MethodGet, "/api/v1/investments/lots?account_id="+strconv.FormatInt(holding.ID, 10)+"&commodity_id="+strconv.FormatInt(instrument.CommodityID, 10), nil)
	lotsRes.AddCookie(f.sessionCookie)
	lotsRec := httptest.NewRecorder()
	handler.ServeHTTP(lotsRec, lotsRes)
	require.Equal(t, http.StatusOK, lotsRec.Code, lotsRec.Body.String())
	var lots investmentLotsResponse
	require.NoError(t, json.NewDecoder(lotsRec.Body).Decode(&lots))
	require.Len(t, lots.Lots, 1)
	assert.Equal(t, "closed", lots.Lots[0].Status)

	positionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/investments/positions", nil)
	positionsReq.AddCookie(f.sessionCookie)
	positionsRec := httptest.NewRecorder()
	handler.ServeHTTP(positionsRec, positionsReq)
	require.Equal(t, http.StatusOK, positionsRec.Code)

	gainsReq := httptest.NewRequest(http.MethodGet, "/api/v1/investments/gains", nil)
	gainsReq.AddCookie(f.sessionCookie)
	gainsRec := httptest.NewRecorder()
	handler.ServeHTTP(gainsRec, gainsReq)
	require.Equal(t, http.StatusOK, gainsRec.Code)
	var gains investmentGainsResponse
	require.NoError(t, json.NewDecoder(gainsRec.Body).Decode(&gains))
	require.Len(t, gains.Realized, 1)
	assert.Equal(t, int64(20000), gains.Realized[0].RealizedGainValue)
}

// --- Error mapping ---

func TestSellInvestment_InsufficientLotsReturnsConflict(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "VWRL")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)

	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy",
		tradeRequestBody(f, holding.ID, instrument.CommodityID, "5", 50000), http.StatusCreated)

	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/sell",
		tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 100000), http.StatusConflict)
	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "CONFLICT", body.Error.Code)
}

func TestBuyInvestment_ValidationFailureReturnsBadRequest(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "VWRL")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)

	body := tradeRequestBody(f, holding.ID, instrument.CommodityID, "0", 100000) // zero quantity
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy", body, http.StatusBadRequest)
	var errBody errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&errBody))
	assert.Equal(t, "VALIDATION_FAILED", errBody.Error.Code)
}

func TestReadInvestmentInstrument_UnknownIDReturnsNotFound(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/investments/instruments/999999", nil)
	req.AddCookie(f.sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, http.StatusNotFound, res.Code)
}

func TestAcceptInvestmentEventSuggestion_AlreadyAcceptedReturnsConflict(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	suggestionID := seedSuggestionForAPITest(t, database, f)

	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/event-suggestions/"+strconv.FormatInt(suggestionID, 10)+"/accept", nil, http.StatusOK)

	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/event-suggestions/"+strconv.FormatInt(suggestionID, 10)+"/accept", nil, http.StatusConflict)
	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "CONFLICT", body.Error.Code)
}

func seedSuggestionForAPITest(t *testing.T, database *sql.DB, f investmentAPITestFixture) int64 {
	t.Helper()
	ctx := context.Background()
	var manualSourceID int64
	require.NoError(t, database.QueryRowContext(ctx, `SELECT id FROM market_data_sources WHERE code = 'manual'`).Scan(&manualSourceID))
	res, err := database.ExecContext(ctx, `
		INSERT INTO investment_provider_events (book_id, source_id, event_family, event_date, status, created_at)
		VALUES (1, ?, 'dividend', '2026-06-01', 'new', '2026-06-01T00:00:00Z')
	`, manualSourceID)
	require.NoError(t, err)
	providerEventID, err := res.LastInsertId()
	require.NoError(t, err)

	proposal := `{"kind":"dividend_income","transaction_date":"2026-06-01","cash_account_id":` +
		strconv.FormatInt(f.cashAccount.ID, 10) + `,"cash_commodity_id":` + strconv.FormatInt(f.commodityID, 10) +
		`,"amount_value":1250,"amount_scale":2,"income_account_id":` + strconv.FormatInt(f.incomeAccount.ID, 10) + `}`

	res, err = database.ExecContext(ctx, `
		INSERT INTO investment_event_suggestions (
			book_id, provider_event_id, confidence_bps, status, proposed_transaction_json, created_at, updated_at
		) VALUES (1, ?, 9000, 'suggested', ?, '2026-06-01T00:00:00Z', '2026-06-01T00:00:00Z')
	`, providerEventID, proposal)
	require.NoError(t, err)
	suggestionID, err := res.LastInsertId()
	require.NoError(t, err)
	return suggestionID
}

// --- Authentication / CSRF ---

func TestInvestmentMutations_RequireCSRFToken(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "VWRL")

	cases := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"create instrument", http.MethodPost, "/api/v1/investments/instruments", investmentInstrumentRequest{CommodityCode: "AAPL", InstrumentType: "stock", DisplayName: "Apple", Symbol: "AAPL", QuantityScale: 6, PriceScale: 4, EffectiveFrom: "2026-01-01"}},
		{"update instrument", http.MethodPatch, "/api/v1/investments/instruments/" + strconv.FormatInt(instrument.ID, 10), investmentInstrumentRequest{CommodityCode: "VWRL", InstrumentType: "etf", DisplayName: "Renamed", Symbol: "VWRL", QuantityScale: 6, PriceScale: 4, EffectiveFrom: "2026-01-01"}},
		{"create holding account", http.MethodPost, "/api/v1/investments/holding-accounts", holdingAccountRequest{InstrumentID: instrument.ID, Name: "Holding", OpenedOn: "2026-01-01", EffectiveFrom: "2026-01-01"}},
		{"buy", http.MethodPost, "/api/v1/investments/buy", tradeRequestBody(f, 1, instrument.CommodityID, "1", 1000)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := json.Marshal(tc.body)
			require.NoError(t, err)
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(string(encoded)))
			req.Header.Set("Content-Type", "application/json")
			setSameOrigin(req)
			req.AddCookie(f.sessionCookie)
			// Deliberately omit the CSRF token header.
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			assert.Equal(t, http.StatusForbidden, res.Code, tc.name)
		})
	}
}

func TestInvestmentEndpoints_RequireAuthentication(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"search", http.MethodGet, "/api/v1/investments/search"},
		{"list instruments", http.MethodGet, "/api/v1/investments/instruments"},
		{"create instrument", http.MethodPost, "/api/v1/investments/instruments"},
		{"positions", http.MethodGet, "/api/v1/investments/positions"},
		{"lots", http.MethodGet, "/api/v1/investments/lots"},
		{"gains", http.MethodGet, "/api/v1/investments/gains"},
		{"buy", http.MethodPost, "/api/v1/investments/buy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			assert.Equal(t, http.StatusUnauthorized, res.Code, tc.name)
		})
	}
}

// --- Handler-level parsing edges ---

func TestReadInvestmentInstrument_NonIntegerPathIDRejected(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/investments/instruments/not-a-number", nil)
	req.AddCookie(f.sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, http.StatusBadRequest, res.Code)
	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "VALIDATION_FAILED", body.Error.Code)
}

func TestCreateInvestmentInstrument_MalformedJSONBodyRejected(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/investments/instruments", strings.NewReader(`{not-valid-json`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, f.csrfToken)
	setSameOrigin(req)
	req.AddCookie(f.sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, http.StatusBadRequest, res.Code)
}

// --- Remaining config/list handlers ---

func TestUpdateInvestmentInstrument_HTTP(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "VWRL")

	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPatch, "/api/v1/investments/instruments/"+strconv.FormatInt(instrument.ID, 10), investmentInstrumentRequest{
		CommodityCode: "VWRL", InstrumentType: "etf", DisplayName: "Vanguard FTSE All-World UCITS ETF", Symbol: "VWRL",
		QuantityScale: 6, PriceScale: 4, EffectiveFrom: "2026-06-01",
		Identifiers: json.RawMessage(`{}`), Metadata: json.RawMessage(`{}`),
	}, http.StatusOK)
	var updated investmentInstrumentResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&updated))
	assert.Equal(t, "Vanguard FTSE All-World UCITS ETF", updated.DisplayName)
}

func TestCostBasisProfiles_ListAndSave_HTTP(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	listRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/investments/cost-basis-profiles", nil, http.StatusOK)
	var listed costBasisProfilesResponse
	require.NoError(t, json.NewDecoder(listRes.Body).Decode(&listed))
	require.Len(t, listed.Profiles, 1, "the default FIFO profile is seeded on first list")

	createRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/cost-basis-profiles", costBasisProfileRequest{
		Name: "ISA Average Cost", Method: "average_cost", Status: "active",
		Metadata: json.RawMessage(`{}`),
	}, http.StatusCreated)
	var created costBasisProfileResponse
	require.NoError(t, json.NewDecoder(createRes.Body).Decode(&created))

	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPatch, "/api/v1/investments/cost-basis-profiles/"+strconv.FormatInt(created.ID, 10), costBasisProfileRequest{
		Name: "ISA Average Cost v2", Method: "average_cost", Status: "active",
		Metadata: json.RawMessage(`{}`),
	}, http.StatusOK)
}

func TestDividendDefaults_ListAndSave_HTTP(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	createRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/dividend-defaults", dividendDefaultRequest{
		IncomeAccountID: f.incomeAccount.ID, Status: "active", EffectiveFrom: "2026-01-01",
		Metadata: json.RawMessage(`{}`),
	}, http.StatusCreated)
	var created dividendDefaultResponse
	require.NoError(t, json.NewDecoder(createRes.Body).Decode(&created))
	assert.Equal(t, f.incomeAccount.ID, created.IncomeAccountID)

	listRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/investments/dividend-defaults", nil, http.StatusOK)
	var listed dividendDefaultsResponse
	require.NoError(t, json.NewDecoder(listRes.Body).Decode(&listed))
	require.Len(t, listed.Defaults, 1)

	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPatch, "/api/v1/investments/dividend-defaults/"+strconv.FormatInt(created.ID, 10), dividendDefaultRequest{
		IncomeAccountID: f.incomeAccount.ID, Status: "archived", EffectiveFrom: "2026-01-01",
		Metadata: json.RawMessage(`{}`),
	}, http.StatusOK)
}

func TestAutomationRules_ListAndReplace_HTTP(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)

	saveRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPut, "/api/v1/investments/automation-rules", investmentAutomationRulesRequest{
		Rules: []investmentAutomationRuleRequest{
			{EventFamily: "dividend", Mode: "suggest", ConfidenceThresholdBPS: 8000, EffectiveFrom: "2026-01-01", RequiredAccounts: json.RawMessage(`{}`)},
		},
	}, http.StatusOK)
	var saved investmentAutomationRulesResponse
	require.NoError(t, json.NewDecoder(saveRes.Body).Decode(&saved))
	require.Len(t, saved.Rules, 1)

	listRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/investments/automation-rules", nil, http.StatusOK)
	var listed investmentAutomationRulesResponse
	require.NoError(t, json.NewDecoder(listRes.Body).Decode(&listed))
	require.Len(t, listed.Rules, 1)

	// An empty-list PUT archives the rule — true replace semantics (Part 1).
	emptyRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPut, "/api/v1/investments/automation-rules", investmentAutomationRulesRequest{}, http.StatusOK)
	var emptied investmentAutomationRulesResponse
	require.NoError(t, json.NewDecoder(emptyRes.Body).Decode(&emptied))
	assert.Empty(t, emptied.Rules)
}

func TestCreateReinvestedDividend_HTTP(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "VWRL")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)

	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/reinvested-dividend", reinvestedDividendRequest{
		TransactionDate: "2026-03-01", CommodityID: instrument.CommodityID, HoldingAccountID: holding.ID,
		IncomeAccountID: &f.incomeAccount.ID, QuantityValue: exact.New(1), QuantityScale: 0,
		AmountValue: 2500, AmountScale: 2, CashCommodityID: f.commodityID,
	}, http.StatusCreated)
	var trade investmentTradeResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&trade))
	require.NotNil(t, trade.LotID)
}

func TestListInvestmentEventsAndSuggestions_HTTP(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	seedSuggestionForAPITest(t, database, f)

	eventsRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/investments/events", nil, http.StatusOK)
	var events investmentProviderEventsResponse
	require.NoError(t, json.NewDecoder(eventsRes.Body).Decode(&events))
	require.Len(t, events.Events, 1)

	suggestionsRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/investments/event-suggestions", nil, http.StatusOK)
	var suggestions investmentEventSuggestionsResponse
	require.NoError(t, json.NewDecoder(suggestionsRes.Body).Decode(&suggestions))
	require.Len(t, suggestions.Suggestions, 1)
	assert.Equal(t, "suggested", suggestions.Suggestions[0].Status)
}
