package api

import (
	"encoding/json"
	"fmt"
	"net/http"
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
	income := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Overview salary","category_type":"income"}`)
	for _, entry := range []struct {
		date     string
		account  int64
		quantity int64
	}{
		{"2026-09-01", income.ID, 10000},
		{"2026-09-07", expense.ID, -3000},
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
	budgetCard := planning.Budget.Data
	require.NotNil(t, budgetCard)
	require.Len(t, budgetCard.Totals, 1)
	assert.Equal(t, []string{"2026-09-01", "2026-09-30"}, []string{budgetCard.StartDate, budgetCard.EndDate})
	assert.Equal(t, 2333, budgetCard.ElapsedBasisPoints)
	assert.Equal(t, "4500", budgetCard.Totals[0].SpentBasisPoints, "posted future expense in the budget month counts; the recurring draft does not")
	assert.Equal(t, app.OverviewAmount{Code: "USD", Value: exact.New(5500), Scale: 2}, budgetCard.Totals[0].Remaining)
	assert.Equal(t, budgetCategoryByID(t, budget, expense.ID).Amounts[0].Remaining.QuantityValue, budgetCard.Totals[0].Remaining.Value)
	require.NotNil(t, planning.Recurring.Data)
	assert.Equal(t, planningRecurringResponse{DraftCount: 1}, *planning.Recurring.Data)

	require.NotNil(t, planning.Forecast.Data)
	require.True(t, planning.Forecast.Data.Complete)
	assert.Equal(t, 90, planning.Forecast.Data.HorizonDays)
	assert.Equal(t, "2026-12-06", planning.Forecast.Data.ThroughDate)
	require.Len(t, planning.Forecast.Data.Minimums, 1)
	minimum := planning.Forecast.Data.Minimums[0]
	assert.Equal(t, "2026-12-06", minimum.Date)
	assert.Equal(t, app.OverviewAmount{Code: "USD", Value: exact.New(-3600), Scale: 2}, minimum.Amount, "70 opening minus the future 15 debit, one carried draft and 90 daily occurrences")
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
		card      sectionOverviewCard[app.ReportHeadline]
		canonical *balanceQuantityResponse
		value     int64
	}{
		{reports.Spending, spending.ConvertedTotal, 3000},
		{reports.Cashflow, cashflow.Buckets[0].ConvertedOperatingNet, 7000},
		{reports.NetWorth, netWorth.Buckets[0].Converted, 7000},
	} {
		require.NotNil(t, row.card.Data)
		require.NotNil(t, row.canonical)
		assert.Equal(t, "ready", row.card.Data.State)
		assert.Equal(t, []app.OverviewAmount{{Code: "USD", Value: exact.New(row.value), Scale: 2}}, row.card.Data.Amounts)
		assert.Equal(t, row.canonical.QuantityValue, row.card.Data.Amounts[0].Value)
		assert.Equal(t, row.canonical.QuantityScale, row.card.Data.Amounts[0].Scale)
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
	require.NotNil(t, planning.Budget.Data)
	require.Len(t, planning.Budget.Data.Totals, 1)
	assert.Equal(t, "1000000000000000045000000", planning.Budget.Data.Totals[0].SpentBasisPoints, "valid over-budget shares can exceed a native integer")
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
	require.NotNil(t, reports.Spending.Data)
	assert.Equal(t, "incomplete", reports.Spending.Data.State)
	assert.Equal(t, []app.OverviewAmount{{Code: "EUR", Value: exact.New(763), Scale: 2}}, reports.Spending.Data.Amounts)
	require.NotNil(t, reports.NetWorth.Data)
	assert.Equal(t, "incomplete", reports.NetWorth.Data.State)
	assert.Empty(t, reports.NetWorth.Data.Amounts, "an unvalued currency must not become a partial net worth")
	require.NotNil(t, reports.GainsRuns.Data)
	assert.Equal(t, []app.OverviewSavedReport{{ID: saved.ID, Kind: "statement", CapturedAt: saved.CapturedAt}}, *reports.GainsRuns.Data)

	_, err := database.Exec(`DROP TABLE budget_targets`)
	require.NoError(t, err)
	var planning planningOverviewResponse
	res = forecastRequest(t, handler, f.sessionCookie, "/api/v1/planning/overview", http.StatusOK)
	require.NoError(t, json.NewDecoder(res.Body).Decode(&planning))
	assert.True(t, planning.Budget.Failed)
	assert.Nil(t, planning.Budget.Data)
	assert.NotNil(t, planning.Recurring.Data)
	require.NotNil(t, planning.Forecast.Data)
	assert.NotEmpty(t, planning.Forecast.Data.Minimums)
}
