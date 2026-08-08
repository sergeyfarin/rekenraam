package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpendingRanksCategoriesAndExcludesTransfers(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, usdID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Spending Checking", "asset", "checking", usdID, 2)
	savings := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Spending Savings", "asset", "savings", usdID, 2)
	groceries := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Spending Groceries","category_type":"expense"}`)
	rent := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Spending Rent","category_type":"expense"}`)
	salary := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Spending Salary","category_type":"income"}`)

	// 300.00 of groceries, 900.00 of rent, 2000.00 of income.
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-03",
		posting(checking.ID, -30000, 2, usdID),
		posting(groceries.ID, 30000, 2, usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-05",
		posting(checking.ID, -90000, 2, usdID),
		posting(rent.ID, 90000, 2, usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-01",
		posting(checking.ID, 200000, 2, usdID),
		posting(salary.ID, -200000, 2, usdID),
	), http.StatusCreated)
	// A transfer between the user's own accounts has no category leg and must
	// never be counted as spending.
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-06",
		posting(checking.ID, -50000, 2, usdID),
		posting(savings.ID, 50000, 2, usdID),
	), http.StatusCreated)

	report := readSpendingForSession(t, handler, sessionCookie, "?start_date=2026-06-01&end_date=2026-06-30")

	assert.Equal(t, "category", report.GroupBy)
	assert.Equal(t, "expense", report.Direction)
	assert.Equal(t, "2026-06-01", report.Query.StartDate)
	assert.Equal(t, "2026-06-30", report.Query.EndDate)

	// 1200.00 total expense: the transfer's 500.00 is absent and the 2000.00
	// of income is not mixed in.
	require.Len(t, report.CommodityTotals, 1)
	assert.Equal(t, "120000", report.CommodityTotals[0].NormalQuantityValue.String())
	assert.Equal(t, usdID, report.RankCommodityID)

	require.Len(t, report.Groups, 2)
	require.NotNil(t, report.Groups[0].CategoryAccountID)
	assert.Equal(t, rent.ID, *report.Groups[0].CategoryAccountID)
	assert.Equal(t, "90000", report.Groups[0].Totals[0].NormalQuantityValue.String())
	assert.Equal(t, int64(7500), report.Groups[0].Totals[0].ShareBasisPoints)
	require.NotNil(t, report.Groups[1].CategoryAccountID)
	assert.Equal(t, groceries.ID, *report.Groups[1].CategoryAccountID)
	assert.Equal(t, "30000", report.Groups[1].Totals[0].NormalQuantityValue.String())
	assert.Equal(t, int64(2500), report.Groups[1].Totals[0].ShareBasisPoints)
}

func TestSpendingIncomeDirectionReportsPositiveMagnitudes(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, usdID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Income Checking", "asset", "checking", usdID, 2)
	salary := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Income Salary","category_type":"income"}`)
	groceries := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Income Groceries","category_type":"expense"}`)

	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-01",
		posting(checking.ID, 200000, 2, usdID),
		posting(salary.ID, -200000, 2, usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-03",
		posting(checking.ID, -30000, 2, usdID),
		posting(groceries.ID, 30000, 2, usdID),
	), http.StatusCreated)

	report := readSpendingForSession(t, handler, sessionCookie, "?start_date=2026-06-01&end_date=2026-06-30&direction=income")

	assert.Equal(t, "income", report.Direction)
	require.Len(t, report.Groups, 1)
	require.NotNil(t, report.Groups[0].CategoryAccountID)
	assert.Equal(t, salary.ID, *report.Groups[0].CategoryAccountID)
	// Income is stored credit-negative; the report presents it as a positive
	// magnitude, and the raw debit-positive value stays available beside it.
	assert.Equal(t, "200000", report.Groups[0].Totals[0].NormalQuantityValue.String())
	assert.Equal(t, "-200000", report.Groups[0].Totals[0].QuantityValue.String())
	// The expense category is absent, not zero-valued.
	assert.Equal(t, int64(10000), report.Groups[0].Totals[0].ShareBasisPoints)
}

func TestSpendingRefundReducesItsCategoryRatherThanRankingSeparately(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, usdID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Refund Checking", "asset", "checking", usdID, 2)
	electronics := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Refund Electronics","category_type":"expense"}`)

	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-03",
		posting(checking.ID, -50000, 2, usdID),
		posting(electronics.ID, 50000, 2, usdID),
	), http.StatusCreated)
	// The item goes back: a credit against the same expense category.
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-09",
		posting(checking.ID, 20000, 2, usdID),
		posting(electronics.ID, -20000, 2, usdID),
	), http.StatusCreated)

	report := readSpendingForSession(t, handler, sessionCookie, "?start_date=2026-06-01&end_date=2026-06-30")

	require.Len(t, report.Groups, 1)
	// 500.00 spent less 200.00 refunded = 300.00 net, not two rows.
	assert.Equal(t, "30000", report.Groups[0].Totals[0].NormalQuantityValue.String())
}

func TestSpendingGroupsByPayeeAndReportsUnlinkedActivityAsUnassigned(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, usdID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Payee Checking", "asset", "checking", usdID, 2)
	groceries := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Payee Groceries","category_type":"expense"}`)
	shop := createPayeeForSession(t, handler, sessionCookie, csrfToken, `{"name":"Corner Shop"}`)

	createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-03",
		"payee_id":`+strconvFormatInt(shop.ID)+`,
		"journal_entries":[{
			"entry_date":"2026-06-03",
			"postings":[
				`+posting(checking.ID, -80000, 2, usdID)+`,
				`+posting(groceries.ID, 80000, 2, usdID)+`
			]
		}]
	}`, http.StatusCreated)
	// No payee at all: must be reported as unassigned, not dropped, or the
	// group totals stop reconciling to commodity_totals.
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-04",
		posting(checking.ID, -20000, 2, usdID),
		posting(groceries.ID, 20000, 2, usdID),
	), http.StatusCreated)

	report := readSpendingForSession(t, handler, sessionCookie, "?start_date=2026-06-01&end_date=2026-06-30&group_by=payee")

	assert.Equal(t, "payee", report.GroupBy)
	require.Len(t, report.Groups, 2)

	require.NotNil(t, report.Groups[0].PayeeID)
	assert.Equal(t, shop.ID, *report.Groups[0].PayeeID)
	assert.Equal(t, "Corner Shop", report.Groups[0].PayeeLabel)
	assert.False(t, report.Groups[0].Unassigned)
	assert.Equal(t, "80000", report.Groups[0].Totals[0].NormalQuantityValue.String())

	assert.True(t, report.Groups[1].Unassigned)
	assert.Nil(t, report.Groups[1].PayeeID)
	assert.Equal(t, "20000", report.Groups[1].Totals[0].NormalQuantityValue.String())

	// The two groups must sum to the report-wide total.
	require.Len(t, report.CommodityTotals, 1)
	assert.Equal(t, "100000", report.CommodityTotals[0].NormalQuantityValue.String())
	assert.Equal(t, int64(10000),
		report.Groups[0].Totals[0].ShareBasisPoints+report.Groups[1].Totals[0].ShareBasisPoints)
}

func TestSpendingExcludesVoidedAndOutOfRangeActivity(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, usdID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Void Checking", "asset", "checking", usdID, 2)
	groceries := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Void Groceries","category_type":"expense"}`)

	kept := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-10",
		posting(checking.ID, -10000, 2, usdID),
		posting(groceries.ID, 10000, 2, usdID),
	), http.StatusCreated)
	voided := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-11",
		posting(checking.ID, -70000, 2, usdID),
		posting(groceries.ID, 70000, 2, usdID),
	), http.StatusCreated)
	mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost,
		"/api/v1/transactions/"+strconvFormatInt(voided.ID)+"/void",
		`{"change_reason":"duplicate import"}`, http.StatusOK)
	// Outside the requested range.
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-07-02",
		posting(checking.ID, -55000, 2, usdID),
		posting(groceries.ID, 55000, 2, usdID),
	), http.StatusCreated)

	report := readSpendingForSession(t, handler, sessionCookie, "?start_date=2026-06-01&end_date=2026-06-30")

	require.Len(t, report.Groups, 1)
	require.NotNil(t, report.Groups[0].CategoryAccountID)
	assert.Equal(t, groceries.ID, *report.Groups[0].CategoryAccountID)
	assert.Equal(t, "10000", report.Groups[0].Totals[0].NormalQuantityValue.String())
	require.NotZero(t, kept.ID)
}

func TestSpendingWithNoActivityReturnsAnEmptyRankingNotAnError(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, _, _ := setupAccountAPITest(t, handler)

	report := readSpendingForSession(t, handler, sessionCookie, "?start_date=2026-06-01&end_date=2026-06-30")

	assert.Empty(t, report.Groups)
	assert.Empty(t, report.CommodityTotals)
	assert.Equal(t, int64(0), report.RankCommodityID)
}

func TestSpendingRejectsInvalidQuery(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, _, _ := setupAccountAPITest(t, handler)

	for _, suffix := range []string{
		"?end_date=2026-06-30",
		"?start_date=2026-06-30&end_date=2026-06-01",
		"?start_date=2026-06-01&end_date=2026-06-30&group_by=tag",
		"?start_date=2026-06-01&end_date=2026-06-30&direction=transfer",
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/spending"+suffix, nil)
		req.AddCookie(sessionCookie)
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		assert.Equal(t, http.StatusBadRequest, res.Code, suffix)
	}
}

func TestSpendingRequiresAuthentication(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/spending?start_date=2026-06-01&end_date=2026-06-30", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	assert.Equal(t, http.StatusUnauthorized, res.Code)
}

func readSpendingForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, suffix string) spendingResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/spending"+suffix, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	var response spendingResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}
