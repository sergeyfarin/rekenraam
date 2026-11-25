package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
	"rekenraam/backend/internal/testdb"
)

func TestSectionOverviewFiguresMatchTheirCanonicalPages(t *testing.T) {
	t.Parallel()
	database, uri := testdb.Open(t)
	instance := newTestInstanceOnDatabase(t, database, uri, HandlerOptions{})
	handler := instance.handler
	clock := func() time.Time { return time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC) }
	instance.services.Settings.SetNowForTest(clock)
	instance.services.Transaction.SetNowForTest(clock)
	cookie, csrf, usd := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, cookie, csrf, "Overview checking", "asset", "checking", usd, 2)
	expense := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Overview groceries","category_type":"expense"}`)
	untargeted := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Overview untargeted expense","category_type":"expense"}`)
	income := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Overview salary","category_type":"income"}`)
	for _, entry := range []struct {
		date     string
		account  int64
		quantity int64
	}{
		{"2026-09-01", income.ID, 10000},
		{"2026-09-07", expense.ID, -3000},
		{"2026-09-07", untargeted.ID, -500},
		{"2026-09-08", expense.ID, -1500},
	} {
		createTransactionForSession(t, handler, cookie, csrf, balancedBody(entry.date,
			posting(checking.ID, entry.quantity, 2, usd), posting(entry.account, -entry.quantity, 2, usd)), http.StatusCreated)
	}
	generateRecurringDraft(t, handler, database, cookie, "2026-09-07", checking.ID, expense.ID, usd, 100)
	budget := mutateBudget(t, handler, cookie, csrf, http.MethodPut,
		fmt.Sprintf("/api/v1/budgets/targets/%d/%d", expense.ID, usd),
		`{"period_start":"2026-09-01","quantity_value":"10000","quantity_scale":2,"change_reason":"monthly plan"}`)

	for _, path := range []string{"/api/v1/planning/overview", "/api/v1/reports/overview"} {
		forecastRequest(t, handler, nil, path, http.StatusUnauthorized)
	}
	var auditBefore, auditAfter, workBefore, workAfter int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&auditBefore))
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM background_work_items`).Scan(&workBefore))
	read := func(path string, data any) {
		t.Helper()
		res := forecastRequest(t, handler, cookie, path, http.StatusOK)
		require.NoError(t, json.NewDecoder(res.Body).Decode(data))
	}
	var planning planningOverviewResponse
	read("/api/v1/planning/overview", &planning)
	assert.Equal(t, "2026-09-07", planning.AsOf)
	budgetCard := planning.Budget.Summary
	require.NotNil(t, budgetCard)
	require.Len(t, budgetCard.Totals, 1)
	assert.Equal(t, []string{"2026-09-01", "2026-09-30"}, []string{budgetCard.StartDate, budgetCard.EndDate})
	assert.Equal(t, 2333, budgetCard.ElapsedBasisPoints)
	assert.Equal(t, "5000", budgetCard.Totals[0].SpentBasisPoints, "the posted future and untargeted expenses count while the recurring draft stays out")
	assert.Equal(t, app.OverviewAmount{Code: "USD", Value: exact.New(5000), Scale: 2}, budgetCard.Totals[0].Remaining)
	expenseTotal := slices.IndexFunc(budget.Totals, func(total budgetTypeTotalResponse) bool {
		return total.CategoryType == "expense" && total.Amount.CommodityID == usd
	})
	require.NotEqual(t, -1, expenseTotal)
	remaining := budget.Totals[expenseTotal].Amount.Remaining
	assert.Equal(t, remaining.QuantityValue, budgetCard.Totals[0].Remaining.Value)
	assert.Equal(t, remaining.QuantityScale, budgetCard.Totals[0].Remaining.Scale)
	require.NotNil(t, planning.Recurring.Summary)
	assert.Equal(t, planningRecurringResponse{DraftCount: 1}, *planning.Recurring.Summary)

	require.NotNil(t, planning.Forecast.Summary)
	require.True(t, planning.Forecast.Summary.Complete)
	assert.Equal(t, 90, planning.Forecast.Summary.HorizonDays)
	assert.Equal(t, "2026-12-06", planning.Forecast.Summary.ThroughDate)
	require.Len(t, planning.Forecast.Summary.Minimums, 1)
	minimum := planning.Forecast.Summary.Minimums[0]
	assert.Equal(t, "2026-12-06", minimum.Date)
	assert.Equal(t, app.OverviewAmount{Code: "USD", Value: exact.New(-4100), Scale: 2}, minimum.Amount, "65 opening minus the future 15 debit, one carried draft and 90 daily occurrences")
	var projection forecastBalancesResponse
	read("/api/v1/forecasts/balances?horizon_days=90", &projection)
	require.Len(t, projection.Totals, 1)
	assert.Equal(t, projection.Totals[0].MinimumDate, minimum.Date)
	assert.Equal(t, projection.Totals[0].MinimumBalance, forecastQuantityResponse{QuantityValue: minimum.Amount.Value, QuantityScale: minimum.Amount.Scale})

	var reports reportsOverviewResponse
	read("/api/v1/reports/overview", &reports)
	assert.Equal(t, "2026-09-07", reports.AsOf)
	assert.Equal(t, "2026-09-01", reports.MonthStart)
	require.NotNil(t, reports.ReportingCurrencyID)
	assert.Equal(t, usd, *reports.ReportingCurrencyID)
	period := fmt.Sprintf("end_date=%s&bucket=month&reporting_currency_id=%d", reports.AsOf, usd)
	var spending spendingResponse
	var cashflow cashflowResponse
	var netWorth netWorthSeriesResponse
	read("/api/v1/reports/spending?group_by=category&mode=spending&start_date="+reports.MonthStart+"&"+period, &spending)
	read("/api/v1/reports/cashflow?start_date="+reports.MonthStart+"&"+period, &cashflow)
	read("/api/v1/reports/net-worth?start_date="+reports.AsOf+"&"+period, &netWorth)
	require.Len(t, cashflow.Buckets, 1)
	require.Len(t, netWorth.Buckets, 1)
	for _, row := range []struct {
		card      readCardResponse[app.ReportHeadline]
		canonical *balanceQuantityResponse
		value     int64
	}{
		{reports.Spending, spending.ConvertedTotal, 3500},
		{reports.Cashflow, cashflow.Buckets[0].ConvertedOperatingNet, 6500},
		{reports.NetWorth, netWorth.Buckets[0].Converted, 6500},
	} {
		require.NotNil(t, row.card.Summary)
		require.NotNil(t, row.canonical)
		assert.Equal(t, "ready", row.card.Summary.State)
		assert.Equal(t, []app.OverviewAmount{{Code: "USD", Value: exact.New(row.value), Scale: 2}}, row.card.Summary.Amounts)
		assert.Equal(t, row.canonical.QuantityValue, row.card.Summary.Amounts[0].Value)
		assert.Equal(t, row.canonical.QuantityScale, row.card.Summary.Amounts[0].Scale)
	}

	require.NoError(t, database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&auditAfter))
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM background_work_items`).Scan(&workAfter))
	assert.Equal(t, auditBefore, auditAfter)
	assert.Equal(t, workBefore, workAfter)

	mutateBudget(t, handler, cookie, csrf, http.MethodPut,
		fmt.Sprintf("/api/v1/budgets/targets/%d/%d", expense.ID, usd),
		`{"period_start":"2026-09-01","quantity_value":"1","quantity_scale":2,"change_reason":"test a very small target"}`)
	createTransactionForSession(t, handler, cookie, csrf, balancedBody("2026-09-07",
		fmt.Sprintf(`{"account_id":%d,"commodity_id":%d,"quantity_value":"-100000000000000000000","quantity_scale":2}`, checking.ID, usd),
		fmt.Sprintf(`{"account_id":%d,"commodity_id":%d,"quantity_value":"100000000000000000000","quantity_scale":2}`, expense.ID, usd)), http.StatusCreated)
	read("/api/v1/planning/overview", &planning)
	require.NotNil(t, planning.Budget.Summary)
	require.Len(t, planning.Budget.Summary.Totals, 1)
	assert.Equal(t, "1000000000000000050000000", planning.Budget.Summary.Totals[0].SpentBasisPoints, "valid over-budget shares can exceed a native integer")
}

func TestSectionOverviewKeepsOtherSourcesWhenBudgetFailsOrRatesAreMissing(t *testing.T) {
	t.Parallel()
	database, uri := testdb.Open(t)
	instance := newTestInstanceOnDatabase(t, database, uri, HandlerOptions{})
	handler := instance.handler
	clock := func() time.Time { return time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC) }
	instance.services.Settings.SetNowForTest(clock)
	instance.services.Transaction.SetNowForTest(clock)
	f := bootstrapInvestmentAPITest(t, handler)
	eur := createCurrencyForSession(t, handler, f.sessionCookie, f.csrfToken, `{"code":"EUR","name":"Euro"}`)
	checking := createLedgerAccount(t, handler, f.sessionCookie, f.csrfToken, "Euro checking", "asset", "checking", eur.ID, 2)
	expense := createCategoryForSession(t, handler, f.sessionCookie, f.csrfToken, `{"name":"Euro expense","category_type":"expense"}`)
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-09-07",
		posting(checking.ID, -763, 2, eur.ID), posting(expense.ID, 763, 2, eur.ID)), http.StatusCreated)
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/gains/runs",
		app.GainsRunRequest{ReportType: "statement", From: "2026-09-01", To: "2026-09-07"}, http.StatusCreated)
	var saved db.GainsRunRecord
	require.NoError(t, json.NewDecoder(res.Body).Decode(&saved))

	var reports reportsOverviewResponse
	res = forecastRequest(t, handler, f.sessionCookie, "/api/v1/reports/overview", http.StatusOK)
	require.NoError(t, json.NewDecoder(res.Body).Decode(&reports))
	require.NotNil(t, reports.Spending.Summary)
	assert.Equal(t, "incomplete", reports.Spending.Summary.State)
	assert.Equal(t, []app.OverviewAmount{{Code: "EUR", Value: exact.New(763), Scale: 2}}, reports.Spending.Summary.Amounts)
	require.NotNil(t, reports.Cashflow.Summary)
	assert.Equal(t, "incomplete", reports.Cashflow.Summary.State)
	assert.Equal(t, []app.OverviewAmount{{Code: "EUR", Value: exact.New(-763), Scale: 2}}, reports.Cashflow.Summary.Amounts)
	require.NotNil(t, reports.NetWorth.Summary)
	assert.Equal(t, "incomplete", reports.NetWorth.Summary.State)
	assert.Empty(t, reports.NetWorth.Summary.Amounts, "an unvalued currency must not become a partial net worth")
	require.NotNil(t, reports.GainsRuns.Summary)
	assert.Equal(t, []app.OverviewSavedReport{{ID: saved.ID, Kind: "statement", CapturedAt: saved.CapturedAt}}, *reports.GainsRuns.Summary)

	_, err := database.Exec(`UPDATE books SET default_currency_commodity_id = NULL WHERE id = 1`)
	require.NoError(t, err)
	_, err = database.Exec(`DROP TABLE investment_gains_runs`)
	require.NoError(t, err)
	res = forecastRequest(t, handler, f.sessionCookie, "/api/v1/reports/overview", http.StatusOK)
	require.NoError(t, json.NewDecoder(res.Body).Decode(&reports))
	assert.Nil(t, reports.ReportingCurrencyID)
	require.NotNil(t, reports.NetWorth.Summary)
	assert.Equal(t, "no_currency", reports.NetWorth.Summary.State)
	assert.Empty(t, reports.NetWorth.Summary.Amounts)
	assert.Equal(t, "failed", reports.GainsRuns.Status)
	assert.Nil(t, reports.GainsRuns.Summary)
	assert.Equal(t, "ok", reports.ReturnsRuns.Status)
	assert.Equal(t, "ok", reports.Spending.Status)

	_, err = database.Exec(`DROP TABLE budget_targets`)
	require.NoError(t, err)
	var planning planningOverviewResponse
	res = forecastRequest(t, handler, f.sessionCookie, "/api/v1/planning/overview", http.StatusOK)
	require.NoError(t, json.NewDecoder(res.Body).Decode(&planning))
	assert.Equal(t, "failed", planning.Budget.Status)
	assert.Nil(t, planning.Budget.Summary)
	assert.NotNil(t, planning.Recurring.Summary)
	require.NotNil(t, planning.Forecast.Summary)
	assert.NotEmpty(t, planning.Forecast.Summary.Minimums)
}
