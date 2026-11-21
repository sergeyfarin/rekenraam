package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
)

func TestHomeReadAuthenticationShapeAndPurity(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v1/home", nil))
	require.Equal(t, http.StatusUnauthorized, res.Code)

	cookie, _, _ := setupAccountAPITest(t, handler)
	var before, after int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&before))
	res = doInvestmentRequest(t, handler, cookie, "", http.MethodGet, "/api/v1/home", nil, http.StatusOK)
	assert.Equal(t, "no-store", res.Header().Get("Cache-Control"))
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&after))
	assert.Equal(t, before, after, "reading Home writes nothing")

	var raw map[string]any
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &raw))
	for _, field := range []string{"as_of", "start_boundary", "reporting_currency", "empty_book", "headlines", "widgets", "commodities"} {
		assert.Contains(t, raw, field)
	}
	var result homeResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &result))
	require.Len(t, result.Headlines, 2)
	assert.Equal(t, "net_worth", result.Headlines[0].ID)
	assert.Equal(t, "cash", result.Headlines[1].ID)
	for _, headline := range raw["headlines"].([]any) {
		item := headline.(map[string]any)
		for _, field := range []string{"value", "start_value", "change", "unavailable", "missing", "stale"} {
			assert.Contains(t, item, field, "null and empty are sent, never omitted")
		}
	}
	widgets := raw["widgets"].(map[string]any)
	for widget, fields := range map[string][]string{
		"spending":        {"start_date", "end_date", "total", "commodity_totals", "categories", "category_count"},
		"budgets":         {"period_start", "period_end", "commodity_id", "other_commodity_count", "total", "categories", "category_count"},
		"accounts":        {"as_of", "open_count", "accounts"},
		"recurring":       {"today", "active_count", "draft_count", "blocked_count", "upcoming"},
		"net_worth_trend": {"start_date", "end_date", "points", "change"},
		"cashflow": {"start_date", "end_date", "cash_scope", "account_count", "inflow", "outflow", "transfer_net", "operating_net",
			"converted_inflow", "converted_outflow", "converted_transfer_net", "converted_operating_net"},
		"holdings":   {"as_of", "position_count", "unpriced_count", "holdings", "missing", "stale"},
		"returns":    {"run_id", "captured_at", "from", "to", "account_scope", "account_count", "time_weighted", "time_weighted_annualized"},
		"allocation": {"run_id", "captured_at", "date", "reason", "basis", "unknown_count", "rows", "row_count"},
		"income":     {"instrument_count", "unassigned_count", "unassigned"},
		"backups":    {"enabled", "next_run_at", "last_success_at", "latest_status", "latest_will_retry"},
	} {
		item := widgets[widget].(map[string]any)
		assert.Equal(t, "ok", item["status"], widget)
		summary, ok := item["summary"].(map[string]any)
		require.True(t, ok, "%s has a summary", widget)
		for _, field := range fields {
			assert.Contains(t, summary, field, "%s.%s is sent, never omitted", widget, field)
		}
	}
}

func TestHomeWidgetAccountLabels(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	var starterID int64
	for _, account := range listAccountsForSession(t, handler, f.sessionCookie, "").Accounts {
		if account.Code == "cash:USD" {
			starterID = account.ID
			break
		}
	}
	require.NotZero(t, starterID, "setup created a starter USD cash account")
	brokerage := createAccountForSession(t, handler, f.sessionCookie, f.csrfToken,
		`{"name":"Brokerage","code":"BRK","account_class":"asset","account_kind":"brokerage","allows_postings":false,"opened_on":"2026-01-01","effective_from":"2026-01-01"}`)
	instrument := createInstrumentForSession(t, handler, f, "HOMELABEL")
	for _, parentID := range []int64{brokerage.ID, starterID} {
		created := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/holding-accounts",
			holdingAccountRequest{InstrumentID: instrument.ID, Name: "Holding", ParentAccountID: &parentID, OpenedOn: "2026-01-01", EffectiveFrom: "2026-01-01"}, http.StatusCreated)
		var holding accountResponse
		require.NoError(t, json.Unmarshal(created.Body.Bytes(), &holding))
		doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy",
			tradeRequestBody(f, holding.ID, instrument.CommodityID, "2", 4000), http.StatusCreated)
	}

	res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/home", nil, http.StatusOK)
	var home struct {
		Widgets struct {
			Accounts struct {
				Summary struct {
					Accounts []map[string]any
				}
			}
			Holdings struct {
				Summary struct {
					Holdings []map[string]any
				}
			}
		}
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &home))
	accounts := make(map[string]map[string]any)
	for _, row := range home.Widgets.Accounts.Summary.Accounts {
		accounts[row["code"].(string)] = row
	}
	starter := accounts["cash:USD"]
	require.NotNil(t, starter)
	assert.Equal(t, "", starter["name"])
	assert.Equal(t, "cash", starter["starter_key"])
	assert.Equal(t, "USD", starter["currency_code"])

	holdings := home.Widgets.Holdings.Summary.Holdings
	require.Len(t, holdings, 2)
	holdingsByBrokerage := make(map[string]map[string]any)
	for _, row := range holdings {
		holdingsByBrokerage[row["brokerage_code"].(string)] = row
	}
	starterHolding := holdingsByBrokerage["cash:USD"]
	require.NotNil(t, starterHolding)
	assert.EqualValues(t, starterID, starterHolding["brokerage_id"])
	assert.Equal(t, "", starterHolding["brokerage_name"])
	assert.Equal(t, "cash", starterHolding["brokerage_starter_key"])
	assert.Equal(t, "USD", starterHolding["brokerage_currency_code"])

	brokerageHolding := holdingsByBrokerage["BRK"]
	require.NotNil(t, brokerageHolding)
	assert.EqualValues(t, brokerage.ID, brokerageHolding["brokerage_id"])
	require.NotNil(t, accounts["BRK"])
	for _, row := range []struct {
		data   map[string]any
		fields []string
	}{
		{accounts["BRK"], []string{"starter_key", "currency_code"}},
		{brokerageHolding, []string{"brokerage_starter_key", "brokerage_currency_code"}},
	} {
		for _, field := range row.fields {
			assert.Contains(t, row.data, field)
			assert.Nil(t, row.data[field], field)
		}
	}
}

// Returns and Allocation show the latest saved run as it was frozen; Home
// calculates no return of its own (#479).
func TestHomeReturnsWidgetsReadTheLatestSavedRun(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "HOMERET")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy",
		tradeRequestBody(f, holding.ID, instrument.CommodityID, "2", 4000), http.StatusCreated)
	priceForReturnsRun(t, handler, f, instrument.CommodityID, "2026-03-31", "25")

	readHome := func() homeResponse {
		res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/home", nil, http.StatusOK)
		var home homeResponse
		require.NoError(t, json.Unmarshal(res.Body.Bytes(), &home))
		return home
	}
	home := readHome()
	require.NotNil(t, home.Widgets.Returns.Summary)
	assert.Nil(t, home.Widgets.Returns.Summary.RunID, "no saved run: nothing, never a zero return")
	assert.Nil(t, home.Widgets.Returns.Summary.TimeWeighted)

	saved := func(from string) db.ReturnsRunRecord {
		request := map[string]any{"from": from, "to": "2026-03-31", "reporting_currency_id": f.commodityID}
		res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/returns/runs", request, http.StatusCreated)
		var run db.ReturnsRunRecord
		require.NoError(t, json.NewDecoder(res.Body).Decode(&run))
		return run
	}
	saved("2026-01-01")
	run := saved("2026-02-01")
	var frozen investmentReturnsResponse
	require.NoError(t, json.Unmarshal(run.OutputJSON, &frozen))

	home = readHome()
	returns := home.Widgets.Returns.Summary
	require.NotNil(t, returns)
	require.NotNil(t, returns.RunID)
	assert.Equal(t, run.ID, *returns.RunID, "the latest run")
	assert.Equal(t, run.CapturedAt, *returns.CapturedAt)
	assert.Equal(t, "2026-02-01", *returns.From)
	assert.Equal(t, frozen.AccountScope, *returns.AccountScope)
	assert.Len(t, frozen.ResolvedAccountIDs, returns.AccountCount)
	assert.Equal(t, frozen.TimeWeighted, *returns.TimeWeighted, "the frozen figure, unchanged")
	assert.Equal(t, frozen.TimeWeightedAnnualized, *returns.TimeWeightedAnnualized)

	allocation := home.Widgets.Allocation.Summary
	require.NotNil(t, allocation)
	assert.Equal(t, run.ID, *allocation.RunID)
	assert.Equal(t, frozen.Allocation.Date, *allocation.Date)
	assert.Equal(t, len(frozen.Allocation.ByAssetType), allocation.RowCount)
	assert.Equal(t, len(frozen.Allocation.Unknown), allocation.UnknownCount)
	require.NotEmpty(t, allocation.Rows)
	for _, row := range allocation.Rows {
		found := false
		for _, source := range frozen.Allocation.ByAssetType {
			if source.AssetType != nil && *source.AssetType == row.AssetType && source.Side == row.Side {
				assert.Equal(t, source.Weight, row.Weight, row.AssetType)
				assert.Equal(t, source.Value, row.Value, row.AssetType)
				found = true
			}
		}
		assert.True(t, found, "%s is a row of the run", row.AssetType)
	}
}
