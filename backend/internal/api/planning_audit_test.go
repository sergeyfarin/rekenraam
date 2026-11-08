package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// Area 8 of #260 (#276). Every expected figure below is derived by hand from
// the fixture comments, never from the code under test.

func budgetAmountFor(t *testing.T, row budgetCategoryResponse, commodityID int64) budgetCategoryAmountResponse {
	t.Helper()
	for _, amount := range row.Amounts {
		if amount.CommodityID == commodityID {
			return amount
		}
	}
	t.Fatalf("category %d has no amount in commodity %d", row.ID, commodityID)
	return budgetCategoryAmountResponse{}
}

func budgetTotalFor(t *testing.T, month budgetMonthResponse, kind string, commodityID int64) budgetCategoryAmountResponse {
	t.Helper()
	for _, total := range month.Totals {
		if total.CategoryType == kind && total.Amount.CommodityID == commodityID {
			return total.Amount
		}
	}
	t.Fatalf("no %s total in commodity %d", kind, commodityID)
	return budgetCategoryAmountResponse{}
}

func TestPlanningAuditBudgetMonthHandDerivedFigures(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	cookie, csrf, usd := setupAccountAPITest(t, handler)
	eur := createCurrencyForSession(t, handler, cookie, csrf, `{"code":"EUR","name":"Euro"}`).ID
	checking := createLedgerAccount(t, handler, cookie, csrf, "Audit checking", "asset", "checking", usd, 2)
	cash := createLedgerAccount(t, handler, cookie, csrf, "Audit wallet", "asset", "cash", usd, 2)
	brokerage := createLedgerAccount(t, handler, cookie, csrf, "Audit brokerage", "asset", "brokerage_cash", usd, 2)
	eurChecking := createLedgerAccount(t, handler, cookie, csrf, "Audit euro checking", "asset", "checking", eur, 2)
	groceries := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Audit groceries","category_type":"expense"}`)
	household := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Audit household","category_type":"expense"}`)
	salary := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Audit salary","category_type":"income"}`)
	tx := func(date string, postings ...string) transactionResponse {
		return createTransactionForSession(t, handler, cookie, csrf, balancedBody(date, postings...), http.StatusCreated)
	}

	// February 2028 (leap year), USD groceries:
	//   12.34 on Feb 1, refund -2.00 on Feb 29, 1.11 on Jan 31 / Mar 1 (outside).
	tx("2028-02-01", posting(checking.ID, -1234, 2, usd), posting(groceries.ID, 1234, 2, usd))
	tx("2028-02-29", posting(checking.ID, 200, 2, usd), posting(groceries.ID, -200, 2, usd))
	tx("2028-01-31", posting(checking.ID, -111, 2, usd), posting(groceries.ID, 111, 2, usd))
	tx("2028-03-01", posting(checking.ID, -111, 2, usd), posting(groceries.ID, 111, 2, usd))
	// Split entry: one on-budget funding leg, two categories, each once.
	tx("2028-02-05", posting(checking.ID, -700, 2, usd), posting(groceries.ID, 300, 2, usd), posting(household.ID, 400, 2, usd))
	// Two on-budget funding legs for one category posting: counted once (10.00),
	// not once per funding account.
	tx("2028-02-06", posting(checking.ID, -500, 2, usd), posting(cash.ID, -500, 2, usd), posting(groceries.ID, 1000, 2, usd))
	// Off-budget by default (brokerage cash): never an actual.
	tx("2028-02-07", posting(brokerage.ID, -999, 2, usd), posting(household.ID, 999, 2, usd))
	// Transfer without a category posting: nothing.
	tx("2028-02-08", posting(checking.ID, -5000, 2, usd), posting(cash.ID, 5000, 2, usd))
	// Euro groceries 7.00 stay in their own currency.
	tx("2028-02-09", posting(eurChecking.ID, -700, 2, eur), posting(groceries.ID, 700, 2, eur))
	// Income 2,500.00 and a 100.00 clawback.
	tx("2028-02-10", posting(checking.ID, 250000, 2, usd), posting(salary.ID, -250000, 2, usd))
	tx("2028-02-11", posting(checking.ID, -10000, 2, usd), posting(salary.ID, 10000, 2, usd))
	// A voided posting and a recurring draft contribute zero.
	voided := tx("2028-02-12", posting(checking.ID, -4444, 2, usd), posting(groceries.ID, 4444, 2, usd))
	mutateTransaction(t, handler, cookie, csrf, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(voided.ID)+"/void", `{"change_reason":"duplicate"}`, http.StatusOK)
	draft := generateRecurringDraft(t, handler, database, cookie, "2028-02-13", checking.ID, groceries.ID, usd, 3333)
	assert.Equal(t, "draft", draft.Status)
	// Treatment switch on the 15th: the 14th counts, the 15th does not.
	tx("2028-02-14", posting(cash.ID, -50, 2, usd), posting(household.ID, 50, 2, usd))
	tx("2028-02-15", posting(cash.ID, -60, 2, usd), posting(household.ID, 60, 2, usd))
	mutateBudget(t, handler, cookie, csrf, http.MethodPut, "/api/v1/budgets/accounts/"+strconvFormatInt(cash.ID),
		`{"effective_from":"2028-02-15","treatment":"off_budget","change_reason":"wallet tracked separately"}`)

	setTarget := func(category, commodity int64, period, value string, scale int) budgetMonthResponse {
		return mutateBudget(t, handler, cookie, csrf, http.MethodPut,
			"/api/v1/budgets/targets/"+strconvFormatInt(category)+"/"+strconvFormatInt(commodity),
			`{"period_start":"`+period+`","quantity_value":"`+value+`","quantity_scale":`+strconvFormatInt(int64(scale))+`,"change_reason":"audit plan"}`)
	}
	setTarget(groceries.ID, usd, "2028-01-01", "10000", 2)
	setTarget(groceries.ID, usd, "2028-02-01", "20", 0)
	setTarget(household.ID, usd, "2028-02-01", "500", 2)
	month := setTarget(salary.ID, usd, "2028-02-01", "300000", 2)

	// Groceries USD: 12.34 - 2.00 + 3.00 + 10.00 = 23.34; target 20 → -3.34.
	g := budgetAmountFor(t, budgetCategoryByID(t, month, groceries.ID), usd)
	assert.Equal(t, "2334", g.Actual.QuantityValue.String())
	assert.Equal(t, 2, g.Actual.QuantityScale)
	assert.Equal(t, "-334", g.Remaining.QuantityValue.String())
	assert.Equal(t, 2, g.Remaining.QuantityScale)
	// Groceries EUR: 7.00, no target, -7.00 remaining; never added to USD.
	ge := budgetAmountFor(t, budgetCategoryByID(t, month, groceries.ID), eur)
	assert.Equal(t, "700", ge.Actual.QuantityValue.String())
	assert.Equal(t, "-700", ge.Remaining.QuantityValue.String())
	// Household USD: 4.00 + 0.50 (Feb 14) = 4.50; target 5.00 → 0.50.
	h := budgetAmountFor(t, budgetCategoryByID(t, month, household.ID), usd)
	assert.Equal(t, "450", h.Actual.QuantityValue.String())
	assert.Equal(t, "50", h.Remaining.QuantityValue.String())
	// Salary: 2,500.00 - 100.00 = 2,400.00; target 3,000.00 → 600.00.
	s := budgetAmountFor(t, budgetCategoryByID(t, month, salary.ID), usd)
	assert.Equal(t, "240000", s.Actual.QuantityValue.String())
	assert.Equal(t, "60000", s.Remaining.QuantityValue.String())
	// Expense totals: USD target 25.00, actual 27.84, remaining -2.84.
	expense := budgetTotalFor(t, month, "expense", usd)
	assert.Equal(t, "2500", expense.Target.QuantityValue.String())
	assert.Equal(t, "2784", expense.Actual.QuantityValue.String())
	assert.Equal(t, "-284", expense.Remaining.QuantityValue.String())
	assert.Equal(t, "700", budgetTotalFor(t, month, "expense", eur).Actual.QuantityValue.String())
	income := budgetTotalFor(t, month, "income", usd)
	assert.Equal(t, "300000", income.Target.QuantityValue.String())
	assert.Equal(t, "240000", income.Actual.QuantityValue.String())

	// January: 1.11 against 100.00 → 98.89; February is not changed by it.
	january := readBudget(t, handler, cookie, "2028-01-01")
	j := budgetAmountFor(t, budgetCategoryByID(t, january, groceries.ID), usd)
	assert.Equal(t, "111", j.Actual.QuantityValue.String())
	assert.Equal(t, "9889", j.Remaining.QuantityValue.String())
	assert.Equal(t, "none", month.RolloverPolicy)
	// March: only the 1.11 on Mar 1, no copied target.
	march := readBudget(t, handler, cookie, "2028-03-01")
	m := budgetAmountFor(t, budgetCategoryByID(t, march, groceries.ID), usd)
	assert.Equal(t, "0", m.Target.QuantityValue.String())
	assert.Equal(t, "111", m.Actual.QuantityValue.String())

	// Posting the recurring draft makes it an ordinary actual: 23.34 + 33.33.
	mutateTransaction(t, handler, cookie, csrf, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(draft.ID)+"/post", `{}`, http.StatusOK)
	posted := budgetAmountFor(t, budgetCategoryByID(t, readBudget(t, handler, cookie, "2028-02-01"), groceries.ID), usd)
	assert.Equal(t, "5667", posted.Actual.QuantityValue.String())

	// Zero removes the target fact; the actual is untouched.
	removed := budgetAmountFor(t, budgetCategoryByID(t, setTarget(groceries.ID, usd, "2028-02-01", "0", 0), groceries.ID), usd)
	assert.Equal(t, "0", removed.Target.QuantityValue.String())
	assert.Equal(t, "5667", removed.Actual.QuantityValue.String())
	var operations []string
	rows, err := database.Query(`SELECT operation FROM audit_events WHERE operation LIKE 'budget.%' ORDER BY id`)
	require.NoError(t, err)
	for rows.Next() {
		var op string
		require.NoError(t, rows.Scan(&op))
		operations = append(operations, op)
	}
	require.NoError(t, rows.Close())
	assert.Equal(t, []string{"budget.account_treatment.set", "budget.target.set", "budget.target.set", "budget.target.set", "budget.target.set", "budget.target.remove"}, operations)
}

func TestPlanningAuditConcurrentBudgetTargetUpsertsLeaveOneFact(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	cookie, csrf, usd := setupAccountAPITest(t, handler)
	category := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Concurrent target","category_type":"expense"}`)
	path := "/api/v1/budgets/targets/" + strconvFormatInt(category.ID) + "/" + strconvFormatInt(usd)
	var group sync.WaitGroup
	statuses := make(chan int, 8)
	for i := range 8 {
		group.Go(func() {
			body := `{"period_start":"2028-02-01","quantity_value":"` + strconvFormatInt(int64(100+i)) + `","quantity_scale":2,"change_reason":"race"}`
			req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(csrfTokenHeader, csrf)
			setSameOrigin(req)
			req.AddCookie(cookie)
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			statuses <- res.Code
		})
	}
	group.Wait()
	close(statuses)
	succeeded := 0
	for status := range statuses {
		if status == http.StatusOK {
			succeeded++
		} else {
			assert.Equal(t, http.StatusServiceUnavailable, status, "only a retryable busy response is acceptable")
		}
	}
	require.Positive(t, succeeded)
	var targets, audits int
	var value string
	var auditID int64
	require.NoError(t, database.QueryRow(`SELECT COUNT(*), MAX(quantity_value), MAX(updated_audit_event_id) FROM budget_targets WHERE category_account_id = ?`, category.ID).Scan(&targets, &value, &auditID))
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE operation = 'budget.target.set'`).Scan(&audits))
	assert.Equal(t, 1, targets, "one current target fact per category, currency and month")
	assert.Equal(t, succeeded, audits, "every committed write has exactly one audit event")
	var auditedLast int64
	require.NoError(t, database.QueryRow(`SELECT MAX(id) FROM audit_events WHERE operation = 'budget.target.set'`).Scan(&auditedLast))
	assert.Equal(t, auditedLast, auditID, "the surviving value is linked to the last committed audit")
	month := readBudget(t, handler, cookie, "2028-02-01")
	assert.Equal(t, value, budgetAmountFor(t, budgetCategoryByID(t, month, category.ID), usd).Target.QuantityValue.String())
}

// Fixture A of the projected-balances plan, shifted to A = 2026-09-07 (the
// API test clock), H = 5, through real HTTP and the real recurring producer.
func TestPlanningAuditForecastFixtureAThroughRealConsumers(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	cookie, csrf, usd := setupAccountAPITest(t, handler)
	eur := createCurrencyForSession(t, handler, cookie, csrf, `{"code":"EUR","name":"Euro"}`).ID
	checking := createLedgerAccount(t, handler, cookie, csrf, "A checking", "asset", "checking", usd, 2)
	savings := createLedgerAccount(t, handler, cookie, csrf, "A savings", "asset", "savings", usd, 2)
	euroCash := createLedgerAccount(t, handler, cookie, csrf, "A euro cash", "asset", "cash", eur, 2)
	card := createLedgerAccount(t, handler, cookie, csrf, "A card", "liability", "credit_card", usd, 2)
	expense := createLedgerAccount(t, handler, cookie, csrf, "A expense", "expense", "expense", usd, 2)
	euroExpense := createLedgerAccount(t, handler, cookie, csrf, "A euro expense", "expense", "expense", eur, 2)
	income := createLedgerAccount(t, handler, cookie, csrf, "A income", "income", "income", usd, 2)
	// Opening money is funded from income accounts outside every cash scope.
	opening := createLedgerAccount(t, handler, cookie, csrf, "A opening", "income", "income", usd, 2)
	euroOpening := createLedgerAccount(t, handler, cookie, csrf, "A euro opening", "income", "income", eur, 2)
	tx := func(date string, postings ...string) transactionResponse {
		return createTransactionForSession(t, handler, cookie, csrf, balancedBody(date, postings...), http.StatusCreated)
	}
	tx("2026-09-01", posting(checking.ID, 100000, 2, usd), posting(savings.ID, 50000, 2, usd), posting(card.ID, -30000, 2, usd), posting(opening.ID, -120000, 2, usd))
	tx("2026-09-01", posting(euroCash.ID, 20000, 2, eur), posting(euroOpening.ID, -20000, 2, eur))
	tx("2026-09-08", posting(checking.ID, 20000, 2, usd), posting(income.ID, -20000, 2, usd))
	tx("2026-09-09", posting(checking.ID, -30000, 2, usd), posting(savings.ID, 30000, 2, usd))
	tx("2026-09-11", posting(checking.ID, -10000, 2, usd), posting(card.ID, 10000, 2, usd))
	// An unrelated euro movement proves currencies never mix.
	tx("2026-09-10", posting(euroCash.ID, -500, 2, eur), posting(euroExpense.ID, 500, 2, eur))

	ownerID, err := db.NewRecurringRepository(database).CurrentBookOwnerID(context.Background(), app.BookID)
	require.NoError(t, err)
	recurring := app.NewRecurringService(db.NewRecurringRepository(database), app.NewTransactionService(
		db.NewTransactionRepository(database), db.NewPayeeRepository(database), db.NewAccountRepository(database), db.NewCommodityRepository(database)),
		app.NewSettingsService(db.NewSettingsRepository(database)))
	clock := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	recurring.SetNowForTest(func() time.Time { return clock })
	template := func(name, date string, lead int, from, to int64, amount int64) app.RecurringTemplate {
		frequency := "daily"
		postings := []db.RecurringTemplatePostingSpec{
			{AccountID: from, CommodityID: usd, QuantityValue: exact.New(-amount), QuantityScale: 2},
			{AccountID: to, CommodityID: usd, QuantityValue: exact.New(amount), QuantityScale: 2},
		}
		created, err := recurring.CreateTemplate(context.Background(), app.WriteRecurringTemplateInput{OwnerUserID: ownerID, Patch: app.RecurringTemplatePatch{
			Name: &name, Frequency: &frequency, StartsOn: &date, EndsOn: app.NullablePatch[string]{Set: true, Value: &date}, LeadDays: &lead, Postings: &postings}})
		require.NoError(t, err)
		return created
	}
	template("Fee", "2026-09-06", 0, checking.ID, expense.ID, 2500)
	rent := template("Rent", "2026-09-09", 5, checking.ID, expense.ID, 10000)
	skipped := template("Skipped", "2026-09-11", 0, checking.ID, expense.ID, 6000)
	salary := template("Salary", "2026-09-12", 0, income.ID, checking.ID, 100000)
	generated, err := recurring.GenerateDue(context.Background(), app.GenerateRecurringInput{OwnerUserID: ownerID})
	require.NoError(t, err)
	assert.Equal(t, app.RecurringGenerationResult{Generated: 2}, generated, "fee (overdue on A) and rent (inside its lead window)")
	require.NoError(t, recurring.SkipOccurrence(context.Background(), app.ReviewRecurringInput{OwnerUserID: ownerID, TemplateID: skipped.ID, OccurrenceDate: "2026-09-11", Reason: "not this month"}))
	occurrences, err := db.NewRecurringRepository(database).ListRecurringOccurrences(context.Background(), app.BookID, rent.ID, 10)
	require.NoError(t, err)
	require.Len(t, occurrences, 1)
	rentDraft := occurrences[0].TransactionID.Int64
	mutateTransaction(t, handler, cookie, csrf, http.MethodPatch, "/api/v1/transactions/"+strconvFormatInt(rentDraft),
		balancedBody("2026-09-10", posting(checking.ID, -12500, 2, usd), posting(expense.ID, 12500, 2, usd)), http.StatusOK)

	path := "/api/v1/forecasts/balances?horizon_days=5"
	before := forecastBalancesFor(t, fixtureFor(handler, cookie), path[len("/api/v1/forecasts/balances"):])
	assert.Equal(t, "2026-09-07", before.AsOfDate)
	assert.Equal(t, "2026-09-08", before.StartDate)
	assert.Equal(t, "2026-09-12", before.EndDate)
	assert.ElementsMatch(t, []int64{checking.ID, savings.ID, euroCash.ID}, before.Scope.ResolvedAccountIDs, "the card is outside the default cash scope")

	type day struct{ posted, draft, template, recorded, projected string }
	usdTotal := forecastTotal(t, before, usd)
	assert.Equal(t, "150000", usdTotal.OpeningBalance.QuantityValue.String())
	want := []day{
		{"20000", "-2500", "0", "170000", "167500"},
		{"0", "0", "0", "170000", "167500"},
		{"0", "-12500", "0", "170000", "155000"},
		{"-10000", "0", "0", "160000", "145000"},
		{"0", "0", "100000", "160000", "245000"},
	}
	require.Len(t, usdTotal.Points, len(want))
	for i, w := range want {
		p := usdTotal.Points[i]
		assert.Equal(t, w, day{p.PostedDelta.QuantityValue.String(), p.DraftDelta.QuantityValue.String(), p.TemplateDelta.QuantityValue.String(),
			p.RecordedBalance.QuantityValue.String(), p.ProjectedBalance.QuantityValue.String()}, "USD aggregate on %s", p.Date)
	}
	assert.Equal(t, "145000", usdTotal.MinimumBalance.QuantityValue.String())
	assert.Equal(t, "2026-09-11", usdTotal.MinimumDate)
	assert.Equal(t, []string{"117500", "87500", "75000", "65000", "165000"}, forecastProjected(forecastAccountSeries(t, before, checking.ID, usd)))
	assert.Equal(t, []string{"50000", "80000", "80000", "80000", "80000"}, forecastProjected(forecastAccountSeries(t, before, savings.ID, usd)))
	assert.Equal(t, []string{"20000", "20000", "19500", "19500", "19500"}, forecastProjected(forecastAccountSeries(t, before, euroCash.ID, eur)))
	assert.Nil(t, forecastAccountSeries(t, before, checking.ID, usd).FirstNegativeDate)
	assert.Nil(t, before.Converted, "no USD+EUR figure exists unless conversion is requested")
	assert.True(t, before.Assumptions.Complete, "a skipped occurrence is not an incomplete assumption")
	assert.Equal(t, 1, before.Assumptions.CarriedForwardEventCount, "only the fee dated Sep 6 is carried to Sep 8")
	// Posted Sep 8 income, Sep 9 transfer, Sep 10 euro expense, Sep 11 card.
	assert.Equal(t, forecastSourceCountsResponse{Posted: 4, Draft: 2, Template: 1}, before.Assumptions.SourceEventCounts)

	// Including the card makes the card payment an internal transfer.
	withCard := forecastBalancesFor(t, fixtureFor(handler, cookie), "?horizon_days=5&account_id="+strconvFormatInt(checking.ID)+"&account_id="+strconvFormatInt(savings.ID)+"&account_id="+strconvFormatInt(card.ID))
	cardTotal := forecastTotal(t, withCard, usd)
	assert.Equal(t, "120000", cardTotal.OpeningBalance.QuantityValue.String())
	assert.Equal(t, "0", cardTotal.Points[3].PostedDelta.QuantityValue.String())
	// 1,200.00 + 200.00 - 25.00 - 125.00 + 1,000.00; the card payment nets to zero.
	assert.Equal(t, "225000", cardTotal.Points[4].ProjectedBalance.QuantityValue.String())

	// Constant-as-of FX: EUR→USD 1.10 on A; a Sep 9 observation of 2.00 lies in
	// the horizon and must not be used. Opening 1,500.00 + 200.00 × 1.10 =
	// 1,720.00; Sep 10's -5.00 converts to -5.50 as its own component.
	price := func(date string, value int64) {
		doInvestmentRequest(t, handler, cookie, csrf, http.MethodPost, "/api/v1/pricing/prices", priceObservationRequest{
			BaseCommodityID: eur, QuoteCommodityID: usd, QuoteType: "manual", AdjustmentBasis: "raw",
			PriceValue: value, PriceScale: 2, ValuationDate: date, IsManual: true,
			Derivation: json.RawMessage(`{}`), SeriesMetadata: json.RawMessage(`{}`), Metadata: json.RawMessage(`{}`)}, http.StatusCreated)
	}
	price("2026-09-07", 110)
	price("2026-09-09", 200)
	// Prices are recorded at wall-clock time, after the forecast's fixed clock;
	// its knowledge cutoff would rightly ignore them. Align them to that clock.
	_, err = database.Exec(`UPDATE price_observations SET recorded_at = '2026-09-07T10:00:00Z'`)
	require.NoError(t, err)
	converted := forecastBalancesFor(t, fixtureFor(handler, cookie), "?horizon_days=5&reporting_currency_id="+strconvFormatInt(usd)+"&fx_method=constant_as_of")
	require.NotNil(t, converted.Valuation)
	require.Truef(t, converted.Valuation.Complete, "%+v", *converted.Valuation)
	require.NotNil(t, converted.Converted)
	assert.Equal(t, "172000", converted.Converted.OpeningBalance.QuantityValue.String())
	assert.Equal(t, []string{"189500", "189500", "176450", "166450", "266450"}, forecastProjected(*converted.Converted))
	assert.Equal(t, "181450", converted.Converted.Points[4].RecordedBalance.QuantityValue.String(), "1,600.00 + 214.50")
	assert.Equal(t, before.Totals, converted.Totals, "conversion never changes the exact per-currency series")

	// Re-running the producer, including concurrently, adds no identity and
	// leaves the forecast byte-identical apart from computed_at.
	var group sync.WaitGroup
	for range 4 {
		group.Go(func() {
			result, err := recurring.GenerateDue(context.Background(), app.GenerateRecurringInput{OwnerUserID: ownerID})
			assert.NoError(t, err)
			assert.Zero(t, result.Generated)
		})
	}
	group.Wait()
	var drafts int
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM recurring_occurrences WHERE status = 'generated'`).Scan(&drafts))
	assert.Equal(t, 2, drafts)
	assertForecastBodyEqual(t, handler, cookie, path, path, "re-running generation changes nothing")

	// Materializing salary unchanged moves its source only.
	clock = time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	generated, err = recurring.GenerateDue(context.Background(), app.GenerateRecurringInput{OwnerUserID: ownerID})
	require.NoError(t, err)
	assert.Equal(t, app.RecurringGenerationResult{Generated: 1}, generated)
	materialized := forecastBalancesFor(t, fixtureFor(handler, cookie), "?horizon_days=5")
	assert.Equal(t, forecastProjected(forecastTotal(t, before, usd)), forecastProjected(forecastTotal(t, materialized, usd)))
	assert.Equal(t, forecastSourceCountsResponse{Posted: 4, Draft: 3, Template: 0}, materialized.Assumptions.SourceEventCounts)
	assert.NotEqual(t, before.BasisToken, materialized.BasisToken)
	occurrences, err = db.NewRecurringRepository(database).ListRecurringOccurrences(context.Background(), app.BookID, salary.ID, 10)
	require.NoError(t, err)
	require.Len(t, occurrences, 1)
	mutateTransaction(t, handler, cookie, csrf, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(occurrences[0].TransactionID.Int64)+"/post", `{}`, http.StatusOK)
	posted := forecastBalancesFor(t, fixtureFor(handler, cookie), "?horizon_days=5")
	postedTotal := forecastTotal(t, posted, usd)
	assert.Equal(t, forecastProjected(forecastTotal(t, before, usd)), forecastProjected(postedTotal), "posting an unchanged future draft keeps the projected curve")
	assert.Equal(t, "260000", postedTotal.Points[4].RecordedBalance.QuantityValue.String(), "recorded-only now includes the posted salary")
	assert.Equal(t, forecastSourceCountsResponse{Posted: 5, Draft: 2, Template: 0}, posted.Assumptions.SourceEventCounts)

	// Model off is byte-identical; the model never edits the core curve.
	assertForecastBodyEqual(t, handler, cookie, path, path+"&spending_model=off", "spending_model=off is the core response")
	on := forecastBalancesFor(t, fixtureFor(handler, cookie), "?horizon_days=5&spending_model=adaptive_v1&history_complete_from=2026-01-01")
	require.NotNil(t, on.LearnedSpending)
	assert.Equal(t, posted.Series, on.Series)
	assert.Equal(t, posted.Totals, on.Totals)
	assert.Equal(t, posted.Assumptions, on.Assumptions)
}

func fixtureFor(handler http.Handler, cookie *http.Cookie) forecastAPIFixture {
	return forecastAPIFixture{handler: handler, session: cookie}
}

func forecastTotal(t *testing.T, body forecastBalancesResponse, commodityID int64) forecastCurrencySeriesResponse {
	t.Helper()
	for _, total := range body.Totals {
		if total.CommodityID == commodityID {
			return total
		}
	}
	t.Fatalf("no total for commodity %d", commodityID)
	return forecastCurrencySeriesResponse{}
}

func forecastAccountSeries(t *testing.T, body forecastBalancesResponse, accountID, commodityID int64) forecastAccountSeriesResponse {
	t.Helper()
	for _, series := range body.Series {
		if series.AccountID == accountID && series.CommodityID == commodityID {
			return series
		}
	}
	t.Fatalf("no series for account %d commodity %d", accountID, commodityID)
	return forecastAccountSeriesResponse{}
}

func forecastProjected[T forecastAccountSeriesResponse | forecastCurrencySeriesResponse](series T) []string {
	var points []forecastPointResponse
	switch s := any(series).(type) {
	case forecastAccountSeriesResponse:
		points = s.Points
	case forecastCurrencySeriesResponse:
		points = s.Points
	}
	out := make([]string, 0, len(points))
	for _, p := range points {
		out = append(out, p.ProjectedBalance.QuantityValue.String())
	}
	return out
}

func assertForecastBodyEqual(t *testing.T, handler http.Handler, cookie *http.Cookie, left, right, message string) {
	t.Helper()
	var a, b map[string]any
	require.NoError(t, json.Unmarshal(forecastRequest(t, handler, cookie, left, http.StatusOK).Body.Bytes(), &a))
	require.NoError(t, json.Unmarshal(forecastRequest(t, handler, cookie, right, http.StatusOK).Body.Bytes(), &b))
	delete(a, "computed_at")
	delete(b, "computed_at")
	assert.Equal(t, a, b, message)
}

// Learned spending with real history: twenty complete Monday–Sunday weeks of
// one 70.00 Wednesday grocery purchase. A posted recurring-linked purchase on a
// Friday must not train the model; its archived template must not disable it.
func TestPlanningAuditLearnedSpendingOnOffThroughRealHistory(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	cookie, csrf, usd := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, cookie, csrf, "L checking", "asset", "checking", usd, 2)
	groceries := createCategoryForSession(t, handler, cookie, csrf, `{"name":"L groceries","category_type":"expense"}`)
	opening := createLedgerAccount(t, handler, cookie, csrf, "L opening", "income", "income", usd, 2)
	createTransactionForSession(t, handler, cookie, csrf, balancedBody("2026-04-01", posting(checking.ID, 200000, 2, usd), posting(opening.ID, -200000, 2, usd)), http.StatusCreated)
	first := time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)
	for week := range 20 {
		date := first.AddDate(0, 0, 7*week).Format(time.DateOnly)
		createTransactionForSession(t, handler, cookie, csrf, balancedBody(date, posting(checking.ID, -7000, 2, usd), posting(groceries.ID, 7000, 2, usd)), http.StatusCreated)
	}
	linked := generateRecurringDraft(t, handler, database, cookie, "2026-08-28", checking.ID, groceries.ID, usd, 3333)
	mutateTransaction(t, handler, cookie, csrf, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(linked.ID)+"/post", `{}`, http.StatusOK)
	var templateID, ownerID int64
	require.NoError(t, database.QueryRow(`SELECT template_id FROM recurring_occurrences WHERE transaction_id = ?`, linked.ID).Scan(&templateID))
	ownerID, err := db.NewRecurringRepository(database).CurrentBookOwnerID(context.Background(), app.BookID)
	require.NoError(t, err)
	recurring := app.NewRecurringService(db.NewRecurringRepository(database), app.NewTransactionService(
		db.NewTransactionRepository(database), db.NewPayeeRepository(database), db.NewAccountRepository(database), db.NewCommodityRepository(database)),
		app.NewSettingsService(db.NewSettingsRepository(database)))
	_, err = recurring.ArchiveTemplate(context.Background(), app.WriteRecurringTemplateInput{OwnerUserID: ownerID, TemplateID: templateID})
	require.NoError(t, err)

	core := "/api/v1/forecasts/balances?horizon_days=7&account_id=" + strconvFormatInt(checking.ID)
	learned := core + "&spending_model=adaptive_v1&history_complete_from=2026-04-20"
	assertForecastBodyEqual(t, handler, cookie, core, core+"&spending_model=off", "off is byte-identical to the core forecast")
	off := forecastBalancesFor(t, fixtureFor(handler, cookie), core[len("/api/v1/forecasts/balances"):])
	on := forecastBalancesFor(t, fixtureFor(handler, cookie), learned[len("/api/v1/forecasts/balances"):])
	assert.Equal(t, off.Series, on.Series)
	assert.Equal(t, off.Totals, on.Totals)
	assert.Equal(t, off.Assumptions, on.Assumptions)
	// 2,000.00 - 20 × 70.00 - 33.33 = 566.67, flat: no saved future item.
	assert.Equal(t, []string{"56667", "56667", "56667", "56667", "56667", "56667", "56667"}, forecastProjected(forecastTotal(t, off, usd)))

	require.NotNil(t, on.LearnedSpending)
	overlay := on.LearnedSpending
	require.Equal(t, "ready", overlay.Status, "reason: %v exclusions: %+v", overlay.Reason, overlay.Exclusions)
	require.Len(t, overlay.Groups, 1)
	group := overlay.Groups[0]
	assert.Equal(t, 20, group.CompletePeriods)
	assert.Equal(t, 20, group.PositivePeriods, "the linked Friday purchase is not a training observation")
	require.Len(t, overlay.Series, 1)
	// The current week (Mon Sep 7 – Sun Sep 13) has no posted grocery spend,
	// so one 70.00 estimate falls on Wednesday Sep 9; Sep 14 starts a new week
	// with no Wednesday inside the horizon.
	var deltas, balances []string
	for _, point := range overlay.Series[0].Points {
		deltas = append(deltas, point.EstimatedDelta.QuantityValue.String())
		balances = append(balances, point.ProjectedBalance.QuantityValue.String())
	}
	assert.Equal(t, []string{"0", "-7000", "0", "0", "0", "0", "0"}, deltas)
	assert.Equal(t, []string{"56667", "49667", "49667", "49667", "49667", "49667", "49667"}, balances)
}

// A recurring draft edited with posting dates projects each leg on its own day.
func TestPlanningAuditForecastProjectsPostingDatedDraftLegs(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	cookie, csrf, usd := setupAccountAPITest(t, handler)
	mutateAccount(t, handler, cookie, csrf, http.MethodPost, "/api/v1/setup/system-accounts", http.StatusCreated)
	checking := createLedgerAccount(t, handler, cookie, csrf, "F checking", "asset", "checking", usd, 2)
	savings := createLedgerAccount(t, handler, cookie, csrf, "F savings", "asset", "savings", usd, 2)
	expense := createCategoryForSession(t, handler, cookie, csrf, `{"name":"F rent","category_type":"expense"}`)
	draft := generateRecurringDraft(t, handler, database, cookie, "2026-09-09", checking.ID, expense.ID, usd, 10000)
	// Edited draft: a transfer whose savings leg posts a day after checking.
	body := `{"transaction_date":"2026-09-09","journal_entries":[{"postings":[
 {"account_id":` + strconvFormatInt(checking.ID) + `,"commodity_id":` + strconvFormatInt(usd) + `,"quantity_value":"-10000","quantity_scale":2,"posted_on":"2026-09-09"},
 {"account_id":` + strconvFormatInt(savings.ID) + `,"commodity_id":` + strconvFormatInt(usd) + `,"quantity_value":"10000","quantity_scale":2,"posted_on":"2026-09-10"}]}]}`
	mutateTransaction(t, handler, cookie, csrf, http.MethodPatch, "/api/v1/transactions/"+strconvFormatInt(draft.ID), body, http.StatusOK)
	result := forecastBalancesFor(t, fixtureFor(handler, cookie), "?horizon_days=5")
	assert.True(t, result.Assumptions.Complete, "%+v", result.Diagnostics)
	// The helper template is daily and open-ended: Sep 10-12 stay template assumptions.
	assert.Equal(t, []string{"0", "-10000", "-20000", "-30000", "-40000"}, forecastProjected(forecastAccountSeries(t, result, checking.ID, usd)))
	assert.Equal(t, []string{"0", "0", "10000", "10000", "10000"}, forecastProjected(forecastAccountSeries(t, result, savings.ID, usd)))
}
