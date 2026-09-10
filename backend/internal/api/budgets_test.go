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
