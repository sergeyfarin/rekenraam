package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBudgetMonthUsesExactPostedActualsAndAccountTreatment(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	cookie, csrf, currencyID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, cookie, csrf, "Budget checking", "asset", "checking", currencyID, 2)
	groceries := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Budget groceries","category_type":"expense"}`)
	salary := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Budget salary","category_type":"income"}`)

	createTransactionForSession(t, handler, cookie, csrf, balancedBody("2028-02-29",
		posting(checking.ID, -1234, 2, currencyID), posting(groceries.ID, 1234, 2, currencyID)), http.StatusCreated)
	createTransactionForSession(t, handler, cookie, csrf, balancedBody("2028-03-01",
		posting(checking.ID, -500, 2, currencyID), posting(groceries.ID, 500, 2, currencyID)), http.StatusCreated)
	voided := createTransactionForSession(t, handler, cookie, csrf, balancedBody("2028-02-20",
		posting(checking.ID, -700, 2, currencyID), posting(groceries.ID, 700, 2, currencyID)), http.StatusCreated)
	mutateTransaction(t, handler, cookie, csrf, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(voided.ID)+"/void", `{"change_reason":"duplicate"}`, http.StatusOK)
	createTransactionForSession(t, handler, cookie, csrf, balancedBody("2028-02-25",
		posting(checking.ID, 5000, 2, currencyID), posting(salary.ID, -5000, 2, currencyID)), http.StatusCreated)

	month := mutateBudget(t, handler, cookie, csrf, http.MethodPut,
		"/api/v1/budgets/targets/"+strconvFormatInt(groceries.ID)+"/"+strconvFormatInt(currencyID),
		`{"period_start":"2028-02-01","quantity_value":"2000","quantity_scale":2,"change_reason":"monthly plan"}`)
	row := budgetCategoryByID(t, month, groceries.ID)
	require.Len(t, row.Amounts, 1)
	assert.Equal(t, "1234", row.Amounts[0].Actual.QuantityValue.String(), "leap-day posting is inside February and March is outside")
	assert.Equal(t, "766", row.Amounts[0].Remaining.QuantityValue.String())
	month = mutateBudget(t, handler, cookie, csrf, http.MethodPut,
		"/api/v1/budgets/targets/"+strconvFormatInt(salary.ID)+"/"+strconvFormatInt(currencyID),
		`{"period_start":"2028-02-01","quantity_value":"6000","quantity_scale":2,"change_reason":"income plan"}`)
	income := budgetCategoryByID(t, month, salary.ID)
	require.Len(t, income.Amounts, 1)
	assert.Equal(t, "5000", income.Amounts[0].Actual.QuantityValue.String(), "credit-normal income is presented as a positive actual")
	assert.Equal(t, "1000", income.Amounts[0].Remaining.QuantityValue.String())
	require.Len(t, month.Totals, 2)
	assert.NotEqual(t, month.Totals[0].CategoryType, month.Totals[1].CategoryType, "income and expense totals are never netted together")

	mutateBudget(t, handler, cookie, csrf, http.MethodPut,
		"/api/v1/budgets/accounts/"+strconvFormatInt(checking.ID),
		`{"effective_from":"2028-02-01","treatment":"off_budget","change_reason":"tracked outside daily plan"}`)
	month = readBudget(t, handler, cookie, "2028-02-01")
	row = budgetCategoryByID(t, month, groceries.ID)
	require.Len(t, row.Amounts, 1)
	assert.Equal(t, "0", row.Amounts[0].Actual.QuantityValue.String(), "off-budget counterpart excludes the category actual")
	assert.Equal(t, "2000", row.Amounts[0].Remaining.QuantityValue.String())
}

func TestBudgetTargetRejectsNonMonthBoundary(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	cookie, csrf, currencyID := setupAccountAPITest(t, handler)
	category := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Boundary category","category_type":"expense"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/budgets/targets/"+strconvFormatInt(category.ID)+"/"+strconvFormatInt(currencyID), strings.NewReader(`{"period_start":"2028-02-02","quantity_value":"1","quantity_scale":0,"change_reason":"bad boundary"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrf)
	setSameOrigin(req)
	req.AddCookie(cookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	assert.Equal(t, http.StatusBadRequest, res.Code)
}

func TestBudgetMonthDefaultsToCurrentOwnerLocalMonth(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	cookie, _, _ := setupAccountAPITest(t, handler)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/budgets/month", nil)
	req.AddCookie(cookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code)
	var month budgetMonthResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&month))
	assert.Equal(t, "2026-09-01", month.PeriodStart)
	assert.Equal(t, "2026-09-30", month.PeriodEnd)
}

func mutateBudget(t *testing.T, handler http.Handler, cookie *http.Cookie, csrf, method, path, body string) budgetMonthResponse {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrf)
	setSameOrigin(req)
	req.AddCookie(cookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equalf(t, http.StatusOK, res.Code, "body: %s", res.Body.String())
	var out budgetMonthResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&out))
	return out
}
func readBudget(t *testing.T, handler http.Handler, cookie *http.Cookie, month string) budgetMonthResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/budgets/month?period_start="+month, nil)
	req.AddCookie(cookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code)
	var out budgetMonthResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&out))
	return out
}
func budgetCategoryByID(t *testing.T, month budgetMonthResponse, id int64) budgetCategoryResponse {
	t.Helper()
	for _, v := range month.Categories {
		if v.ID == id {
			return v
		}
	}
	t.Fatalf("category %d not found", id)
	return budgetCategoryResponse{}
}

// #319: posting-dated splits bridge their funding legs through
// transfer_clearing; the bridged category entry is still on-budget spending.
func TestBudgetCountsPostingDatedSplitsFundedThroughClearing(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	cookie, csrf, usd := setupAccountAPITest(t, handler)
	mutateAccount(t, handler, cookie, csrf, http.MethodPost, "/api/v1/setup/system-accounts", http.StatusCreated)
	checking := createLedgerAccount(t, handler, cookie, csrf, "Dated checking", "asset", "checking", usd, 2)
	card := createLedgerAccount(t, handler, cookie, csrf, "Dated card", "liability", "credit_card", usd, 2)
	brokerage := createLedgerAccount(t, handler, cookie, csrf, "Dated brokerage", "asset", "brokerage_cash", usd, 2)
	other := createLedgerAccount(t, handler, cookie, csrf, "Dated other", "asset", "other_asset", usd, 2)
	groceries := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Dated groceries","category_type":"expense"}`)
	leg := func(account int64, value, postedOn string) string {
		dated := ""
		if postedOn != "" {
			dated = `,"posted_on":"` + postedOn + `"`
		}
		return `{"account_id":` + strconvFormatInt(account) + `,"commodity_id":` + strconvFormatInt(usd) + `,"quantity_value":"` + value + `","quantity_scale":2` + dated + `}`
	}
	split := func(date string, legs ...string) {
		createTransactionForSession(t, handler, cookie, csrf, `{"transaction_date":"`+date+`","journal_entries":[{"postings":[`+strings.Join(legs, ",")+`]}]}`, http.StatusCreated)
	}
	// 50.00 split dated May 10; checking posts May 11, card May 12.
	split("2026-05-10", leg(checking.ID, "-15000", "2026-05-11"), leg(card.ID, "10000", "2026-05-12"), leg(groceries.ID, "5000", ""))
	// 7.00 dated May 31 whose funding posts June 1 belongs to May.
	split("2026-05-31", leg(checking.ID, "-1400", "2026-06-01"), leg(card.ID, "700", "2026-06-02"), leg(groceries.ID, "700", ""))
	// Off-budget-only funding stays outside the budget: 9.00.
	split("2026-05-20", leg(brokerage.ID, "-1800", "2026-05-21"), leg(other.ID, "900", "2026-05-22"), leg(groceries.ID, "900", ""))

	may := budgetCategoryByID(t, readBudget(t, handler, cookie, "2026-05-01"), groceries.ID)
	require.Len(t, may.Amounts, 1)
	assert.Equal(t, "5700", may.Amounts[0].Actual.QuantityValue.String(), "50.00 + 7.00; the off-budget 9.00 is excluded")
	june := budgetCategoryByID(t, readBudget(t, handler, cookie, "2026-06-01"), groceries.ID)
	assert.Empty(t, june.Amounts, "the bridged funding day does not move the category month")
}

// #320: archiving a category later must not rewrite a month it was active in.
// The category clock is 2026-01-01, so the disables below take effect then and
// the active month is December 2025: a disable dated before a posting the
// category has is refused (#664).
func TestBudgetKeepsArchivedCategoryHistoryInItsMonths(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	cookie, csrf, usd := setupAccountAPITest(t, handler)
	checking := createLedgerAccountOpenedOn(t, handler, cookie, csrf, "Archive checking", "asset", "checking", usd, 2, "2025-01-01")
	kept := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Archive kept","category_type":"expense"}`)
	retired := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Archive retired","category_type":"expense"}`)
	bonus := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Archive bonus","category_type":"income"}`)
	unused := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Archive unused","category_type":"expense"}`)
	createTransactionForSession(t, handler, cookie, csrf, balancedBody("2025-12-10", posting(checking.ID, -1000, 2, usd), posting(kept.ID, 1000, 2, usd)), http.StatusCreated)
	createTransactionForSession(t, handler, cookie, csrf, balancedBody("2025-12-11", posting(checking.ID, -999, 2, usd), posting(retired.ID, 999, 2, usd)), http.StatusCreated)
	createTransactionForSession(t, handler, cookie, csrf, balancedBody("2025-12-12", posting(checking.ID, 30000, 2, usd), posting(bonus.ID, -30000, 2, usd)), http.StatusCreated)
	mutateBudget(t, handler, cookie, csrf, http.MethodPut, "/api/v1/budgets/targets/"+strconvFormatInt(retired.ID)+"/"+strconvFormatInt(usd),
		`{"period_start":"2025-12-01","quantity_value":"1500","quantity_scale":2,"change_reason":"plan"}`)
	for _, id := range []int64{retired.ID, bonus.ID, unused.ID} {
		mutateCategory(t, handler, cookie, csrf, http.MethodPost, "/api/v1/categories/"+strconvFormatInt(id)+"/disable", `{}`, http.StatusOK)
	}

	month := readBudget(t, handler, cookie, "2025-12-01")
	expense := budgetTotalFor(t, month, "expense", usd)
	assert.Equal(t, "1999", expense.Actual.QuantityValue.String(), "10.00 + 9.99")
	assert.Equal(t, "1500", expense.Target.QuantityValue.String())
	assert.Equal(t, "30000", budgetTotalFor(t, month, "income", usd).Actual.QuantityValue.String(), "archived income keeps its positive sign")
	row := budgetCategoryByID(t, month, retired.ID)
	assert.Equal(t, "archived", row.Status)
	require.Len(t, row.Amounts, 1)
	assert.Equal(t, "999", row.Amounts[0].Actual.QuantityValue.String())
	assert.Equal(t, "501", row.Amounts[0].Remaining.QuantityValue.String())
	assert.Equal(t, "active", budgetCategoryByID(t, month, kept.ID).Status)
	for _, c := range month.Categories {
		assert.NotEqual(t, unused.ID, c.ID, "an archived category with no activity stays hidden")
	}
	for _, c := range readBudget(t, handler, cookie, "2026-02-01").Categories {
		assert.NotEqual(t, retired.ID, c.ID, "an archived category is hidden from months without activity")
	}
}
