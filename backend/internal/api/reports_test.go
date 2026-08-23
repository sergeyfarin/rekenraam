package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spendingFixture is the deterministic multi-account/multi-commodity basis the
// reports plan asks for: one checking account under a parent, a credit card, an
// expense and income category, a refund, a pure transfer, a voided expense, and
// a EUR expense.
type spendingFixture struct {
	sessionCookie *http.Cookie
	csrfToken     string
	usdID         int64
	eurID         int64
	cashParent    accountResponse
	checking      accountResponse
	card          accountResponse
	euroCash      accountResponse
	groceries     categoryResponse
	travel        categoryResponse
	salary        categoryResponse
}

func newSpendingFixture(t *testing.T, handler http.Handler) spendingFixture {
	t.Helper()

	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")
	currencySetup := completeCurrencySetupForSession(t, handler, sessionCookie, csrfToken, "USD", []setupCurrencySelectionRequest{
		{Code: "USD", Name: "US Dollar"},
	})
	usdID := currencySetup.DefaultCurrency.ID
	eurID := createCurrencyForSession(t, handler, sessionCookie, csrfToken, `{"code":"EUR","name":"Euro"}`).ID
	require.NotZero(t, eurID)

	mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/setup/system-accounts", http.StatusCreated)

	cashParent := createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Cash Parent",
		"account_class":"asset",
		"account_kind":"other_asset",
		"allows_postings":false,
		"opened_on":"2026-01-01",
		"effective_from":"2026-01-01"
	}`)
	checking := createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Checking",
		"account_class":"asset",
		"account_kind":"checking",
		"parent_account_id":`+strconvFormatInt(cashParent.ID)+`,
		"default_commodity_id":`+strconvFormatInt(usdID)+`,
		"quantity_scale_override":2,
		"allows_postings":true,
		"opened_on":"2026-01-01",
		"effective_from":"2026-01-01"
	}`)
	card := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Credit Card", "liability", "credit_card", usdID, 2)
	euroCash := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Euro Cash", "asset", "cash", eurID, 2)

	marketHall := createPayeeForSession(t, handler, sessionCookie, csrfToken, `{"name":"Market Hall"}`)

	groceries := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Groceries","category_type":"expense"}`)
	travel := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Travel","category_type":"expense"}`)
	salary := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Salary","category_type":"income"}`)

	// 120.00 groceries from checking, with a payee.
	createTransactionForSession(t, handler, sessionCookie, csrfToken, payeeBody("2026-06-03", marketHall.ID,
		posting(checking.ID, -12000, 2, usdID),
		posting(groceries.ID, 12000, 2, usdID),
	), http.StatusCreated)
	// 20.00 grocery refund, same payee: reduces the grocery total.
	createTransactionForSession(t, handler, sessionCookie, csrfToken, payeeBody("2026-06-05", marketHall.ID,
		posting(checking.ID, 2000, 2, usdID),
		posting(groceries.ID, -2000, 2, usdID),
	), http.StatusCreated)
	// 300.00 travel on the card, no payee recorded.
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(card.ID, -30000, 2, usdID),
		posting(travel.ID, 30000, 2, usdID),
	), http.StatusCreated)
	// 2000.00 salary into checking.
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-01",
		posting(checking.ID, 200000, 2, usdID),
		posting(salary.ID, -200000, 2, usdID),
	), http.StatusCreated)
	// A pure transfer: no category posting, so it can never enter the basis.
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-09",
		posting(checking.ID, -5000, 2, usdID),
		posting(card.ID, 5000, 2, usdID),
	), http.StatusCreated)
	// A voided expense must not count.
	voided := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-10",
		posting(checking.ID, -9900, 2, usdID),
		posting(groceries.ID, 9900, 2, usdID),
	), http.StatusCreated)
	mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost,
		"/api/v1/transactions/"+strconvFormatInt(voided.ID)+"/void",
		`{"change_reason":"reports fixture"}`, http.StatusOK)
	// 50.00 EUR travel: a second commodity that must never be summed with USD.
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-11",
		posting(euroCash.ID, -5000, 2, eurID),
		posting(travel.ID, 5000, 2, eurID),
	), http.StatusCreated)
	// Outside the reported range.
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-07-02",
		posting(checking.ID, -7700, 2, usdID),
		posting(groceries.ID, 7700, 2, usdID),
	), http.StatusCreated)

	return spendingFixture{
		sessionCookie: sessionCookie,
		csrfToken:     csrfToken,
		usdID:         usdID,
		eurID:         eurID,
		cashParent:    cashParent,
		checking:      checking,
		card:          card,
		euroCash:      euroCash,
		groceries:     groceries,
		travel:        travel,
		salary:        salary,
	}
}

func TestSpendingReportRanksExpenseCategoriesAndNetsRefunds(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	fixture := newSpendingFixture(t, handler)

	report := readSpendingForSession(t, handler, fixture.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&group_by=category")

	require.Len(t, report.Groups, 2)
	// Travel (300.00 USD) outranks groceries (120.00 - 20.00 = 100.00 USD).
	assert.Equal(t, fixture.travel.ID, *report.Groups[0].CategoryID)
	assert.Equal(t, "expense", report.Groups[0].CategoryType)
	assertGroupTotal(t, report.Groups[0], fixture.usdID, "30000", 2)
	assert.Equal(t, fixture.groceries.ID, *report.Groups[1].CategoryID)
	assertGroupTotal(t, report.Groups[1], fixture.usdID, "10000", 2)

	// EUR travel stays on its own commodity row and is never folded into USD.
	assertGroupTotal(t, report.Groups[0], fixture.eurID, "5000", 2)

	// Commodity totals are per commodity: 400.00 USD and 50.00 EUR.
	assertBalance(t, report.CommodityTotals, fixture.usdID, 40000, 2, 40000)
	assertBalance(t, report.CommodityTotals, fixture.eurID, 5000, 2, 5000)

	// Shares are within-commodity only: 300/400 = 75%, 100/400 = 25%.
	assert.Equal(t, int64(7500), *groupTotal(t, report.Groups[0], fixture.usdID).ShareBasisPoints)
	assert.Equal(t, int64(2500), *groupTotal(t, report.Groups[1], fixture.usdID).ShareBasisPoints)
	// The only EUR spender holds 100% of EUR.
	assert.Equal(t, int64(10000), *groupTotal(t, report.Groups[0], fixture.eurID).ShareBasisPoints)

	assert.Equal(t, "direct_postings", report.GroupingPolicy)
	assert.Equal(t, "spending", report.Query.Mode)
}

func TestSpendingReportGroupsByPayeeIncludingUnattributedPostings(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	fixture := newSpendingFixture(t, handler)

	report := readSpendingForSession(t, handler, fixture.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&group_by=payee")

	require.Len(t, report.Groups, 2)
	// The card travel has no payee: it must still appear, or the ranking would
	// not reconcile to the commodity totals.
	unattributed := spendingGroupWithoutPayee(t, report.Groups)
	assert.Empty(t, unattributed.Name)
	assertGroupTotal(t, unattributed, fixture.usdID, "30000", 2)

	named := spendingGroupWithPayee(t, report.Groups)
	assert.Equal(t, "Market Hall", named.Name)
	assertGroupTotal(t, named, fixture.usdID, "10000", 2)

	// Groups reconcile exactly to the commodity total for every commodity.
	assertGroupsReconcileToCommodityTotals(t, report)
}

func TestSpendingReportExcludesTransfersVoidedAndOutOfRangePostings(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	fixture := newSpendingFixture(t, handler)

	report := readSpendingForSession(t, handler, fixture.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&group_by=category")

	// The 99.00 voided grocery expense and the 77.00 July expense are both out
	// of the basis, so groceries stays at exactly 100.00.
	assertGroupTotal(t, spendingGroupForCategory(t, report.Groups, fixture.groceries.ID), fixture.usdID, "10000", 2)
	// The checking-to-card transfer has no category posting and cannot appear.
	assertBalance(t, report.CommodityTotals, fixture.usdID, 40000, 2, 40000)
	assert.Contains(t, report.ExcludedSystemRoles, "commodity_trading")
}

func TestSpendingReportIncomeModeReportsPositiveMagnitudes(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	fixture := newSpendingFixture(t, handler)

	report := readSpendingForSession(t, handler, fixture.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&group_by=category&mode=income")

	require.Len(t, report.Groups, 1)
	assert.Equal(t, fixture.salary.ID, *report.Groups[0].CategoryID)
	assert.Equal(t, "income", report.Groups[0].CategoryType)
	// Income is credit-normal in the ledger; the report presents +2000.00.
	assertGroupTotal(t, report.Groups[0], fixture.usdID, "200000", 2)
	assertBalance(t, report.CommodityTotals, fixture.usdID, 200000, 2, 200000)
	assert.Equal(t, "income", report.Query.Mode)
}

func TestSpendingReportCounterpartAccountFilterResolvesDescendants(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	fixture := newSpendingFixture(t, handler)

	// Checking directly: groceries only. The card travel is excluded.
	direct := readSpendingForSession(t, handler, fixture.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&group_by=category&account_id="+strconvFormatInt(fixture.checking.ID))
	require.Len(t, direct.Groups, 1)
	assert.Equal(t, fixture.groceries.ID, *direct.Groups[0].CategoryID)
	assertGroupTotal(t, direct.Groups[0], fixture.usdID, "10000", 2)

	// The non-posting parent alone matches nothing without descendants.
	parentOnly := readSpendingForSession(t, handler, fixture.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&group_by=category&account_id="+strconvFormatInt(fixture.cashParent.ID))
	assert.Empty(t, parentOnly.Groups)

	// With descendants the parent picks checking's activity up.
	subtree := readSpendingForSession(t, handler, fixture.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&group_by=category&include_descendants=true&account_id="+strconvFormatInt(fixture.cashParent.ID))
	require.Len(t, subtree.Groups, 1)
	assertGroupTotal(t, subtree.Groups[0], fixture.usdID, "10000", 2)
	assert.Contains(t, subtree.Query.Filters.ResolvedAccountIDs, fixture.checking.ID)
	assert.Equal(t, []int64{fixture.cashParent.ID}, subtree.Query.Filters.AccountIDs)
	assert.True(t, subtree.Query.Filters.IncludeDescendants)
}

func TestSpendingReportCommodityFilterKeepsOneCommodity(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	fixture := newSpendingFixture(t, handler)

	report := readSpendingForSession(t, handler, fixture.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&group_by=category&commodity_id="+strconvFormatInt(fixture.eurID))

	require.Len(t, report.Groups, 1)
	assert.Equal(t, fixture.travel.ID, *report.Groups[0].CategoryID)
	require.Len(t, report.Groups[0].Totals, 1)
	assert.Equal(t, fixture.eurID, report.Groups[0].Totals[0].CommodityID)
	assertBalance(t, report.CommodityTotals, fixture.eurID, 5000, 2, 5000)
}

func TestSpendingReportRejectsInvalidQuery(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	fixture := newSpendingFixture(t, handler)

	for name, suffix := range map[string]string{
		"missing end date":     "?start_date=2026-06-01&group_by=category",
		"missing group by":     "?start_date=2026-06-01&end_date=2026-06-30",
		"inverted range":       "?start_date=2026-06-30&end_date=2026-06-01&group_by=category",
		"unknown group by":     "?start_date=2026-06-01&end_date=2026-06-30&group_by=commodity",
		"unknown mode":         "?start_date=2026-06-01&end_date=2026-06-30&group_by=category&mode=savings",
		"unparsable id":        "?start_date=2026-06-01&end_date=2026-06-30&group_by=category&account_id=abc",
		"inaccessible account": "?start_date=2026-06-01&end_date=2026-06-30&group_by=category&account_id=999999",
		"inaccessible payee":   "?start_date=2026-06-01&end_date=2026-06-30&group_by=payee&payee_id=999999",
		"unknown commodity":    "?start_date=2026-06-01&end_date=2026-06-30&group_by=category&commodity_id=999999",
		"bad boolean":          "?start_date=2026-06-01&end_date=2026-06-30&group_by=category&include_descendants=maybe",
		"non positive id":      "?start_date=2026-06-01&end_date=2026-06-30&group_by=category&account_id=0",
		"malformed date":       "?start_date=06-01-2026&end_date=2026-06-30&group_by=category",
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/spending"+suffix, nil)
		req.AddCookie(fixture.sessionCookie)
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		assert.Equal(t, http.StatusBadRequest, res.Code, name)
	}
}

func TestSpendingReportRequiresAuthentication(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/spending?start_date=2026-06-01&end_date=2026-06-30&group_by=category", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	assert.Equal(t, http.StatusUnauthorized, res.Code)
}

func TestNetWorthSeriesAccountAndCommodityFiltersNarrowTheSeries(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	fixture := newSpendingFixture(t, handler)

	unfiltered := readNetWorthSeriesForSession(t, handler, fixture.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&bucket=month")
	require.Len(t, unfiltered.Buckets, 1)
	// Checking: +2000.00 - 120.00 + 20.00 - 50.00 transfer = 1850.00.
	// Card: +50.00 - 300.00 = -250.00. Net USD = 1600.00.
	assertBalance(t, unfiltered.Buckets[0].Totals, fixture.usdID, 160000, 2, 160000)
	assertBalance(t, unfiltered.Buckets[0].Totals, fixture.eurID, -5000, 2, -5000)

	checkingOnly := readNetWorthSeriesForSession(t, handler, fixture.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&bucket=month&account_id="+strconvFormatInt(fixture.checking.ID))
	require.Len(t, checkingOnly.Buckets, 1)
	assertBalance(t, checkingOnly.Buckets[0].Totals, fixture.usdID, 185000, 2, 185000)
	assert.Equal(t, []int64{fixture.checking.ID}, checkingOnly.Query.Filters.AccountIDs)

	// The non-posting parent contributes nothing until descendants are resolved.
	parentOnly := readNetWorthSeriesForSession(t, handler, fixture.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&bucket=month&account_id="+strconvFormatInt(fixture.cashParent.ID))
	assert.Empty(t, parentOnly.Buckets[0].Totals)

	subtree := readNetWorthSeriesForSession(t, handler, fixture.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&bucket=month&include_descendants=true&account_id="+strconvFormatInt(fixture.cashParent.ID))
	assertBalance(t, subtree.Buckets[0].Totals, fixture.usdID, 185000, 2, 185000)

	usdOnly := readNetWorthSeriesForSession(t, handler, fixture.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&bucket=month&commodity_id="+strconvFormatInt(fixture.usdID))
	require.Len(t, usdOnly.Buckets[0].Totals, 1)
	assertBalance(t, usdOnly.Buckets[0].Totals, fixture.usdID, 160000, 2, 160000)
}

func TestNetWorthSeriesRejectsInvalidFilters(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	fixture := newSpendingFixture(t, handler)

	for name, suffix := range map[string]string{
		"inaccessible account": "?start_date=2026-06-01&end_date=2026-06-30&bucket=month&account_id=999999",
		"unknown commodity":    "?start_date=2026-06-01&end_date=2026-06-30&bucket=month&commodity_id=999999",
		"unparsable id":        "?start_date=2026-06-01&end_date=2026-06-30&bucket=month&account_id=abc",
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/net-worth"+suffix, nil)
		req.AddCookie(fixture.sessionCookie)
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		assert.Equal(t, http.StatusBadRequest, res.Code, name)
	}
}

// payeeBody references a payee by id. A bare payee_name is free text on the
// transaction version and never sets payee_id, so it would not group.
func payeeBody(transactionDate string, payeeID int64, postings ...string) string {
	return `{
		"transaction_date":"` + transactionDate + `",
		"payee_id":` + strconvFormatInt(payeeID) + `,
		"journal_entries":[{
			"entry_date":"` + transactionDate + `",
			"postings":[` + strings.Join(postings, ",") + `]
		}]
	}`
}

func readSpendingForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, suffix string) spendingResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/spending"+suffix, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	var response spendingResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}

func groupTotal(t *testing.T, group spendingGroupResponse, commodityID int64) spendingGroupTotalResponse {
	t.Helper()

	for _, total := range group.Totals {
		if total.CommodityID == commodityID {
			return total
		}
	}
	t.Fatalf("group has no total for commodity %d", commodityID)
	return spendingGroupTotalResponse{}
}

func assertGroupTotal(t *testing.T, group spendingGroupResponse, commodityID int64, wantValue string, wantScale int) {
	t.Helper()

	total := groupTotal(t, group, commodityID)
	assert.Equal(t, wantValue, total.QuantityValue.String())
	assert.Equal(t, wantScale, total.QuantityScale)
}

func spendingGroupForCategory(t *testing.T, groups []spendingGroupResponse, categoryID int64) spendingGroupResponse {
	t.Helper()

	for _, group := range groups {
		if group.CategoryID != nil && *group.CategoryID == categoryID {
			return group
		}
	}
	t.Fatalf("no group for category %d", categoryID)
	return spendingGroupResponse{}
}

func spendingGroupWithPayee(t *testing.T, groups []spendingGroupResponse) spendingGroupResponse {
	t.Helper()

	for _, group := range groups {
		if group.PayeeID != nil {
			return group
		}
	}
	t.Fatal("expected a group with a payee")
	return spendingGroupResponse{}
}

func spendingGroupWithoutPayee(t *testing.T, groups []spendingGroupResponse) spendingGroupResponse {
	t.Helper()

	for _, group := range groups {
		if group.PayeeID == nil {
			return group
		}
	}
	t.Fatal("expected the unattributed group")
	return spendingGroupResponse{}
}

// assertGroupsReconcileToCommodityTotals is the invariant that makes the ranking
// trustworthy: every commodity's group magnitudes must sum exactly to that
// commodity's reported total, with nothing silently dropped.
func assertGroupsReconcileToCommodityTotals(t *testing.T, report spendingResponse) {
	t.Helper()

	sums := map[int64]int64{}
	for _, group := range report.Groups {
		for _, total := range group.Totals {
			require.Equal(t, 2, total.QuantityScale, "fixture uses scale 2 throughout")
			sums[total.CommodityID] += mustParseInt64(t, total.QuantityValue.String())
		}
	}
	for _, total := range report.CommodityTotals {
		require.Equal(t, 2, total.QuantityScale)
		assert.Equal(t, mustParseInt64(t, total.QuantityValue.String()), sums[total.CommodityID],
			"groups must reconcile to the commodity total for commodity %d", total.CommodityID)
	}
}

func mustParseInt64(t *testing.T, value string) int64 {
	t.Helper()

	parsed, err := strconv.ParseInt(value, 10, 64)
	require.NoError(t, err)
	return parsed
}

// The series is the same number as the point-in-time endpoint, one per bucket
// end. T-51 replaced the per-bucket ledger read with a single read folded
// forward, so this pins the two against each other rather than against
// hand-computed constants: a fold that drifts, double-counts, or leaks one
// bucket's state into the next shows up here.
func TestNetWorthSeriesMatchesPointInTimeNetWorthAtEveryBucketEnd(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, usdID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Fold Checking", "asset", "checking", usdID, 2)
	card := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Fold Card", "liability", "credit_card", usdID, 2)
	salary := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Fold Salary","category_type":"income"}`)
	groceries := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Fold Groceries","category_type":"expense"}`)

	// Deliberately created out of date order, and with two entries on one date,
	// so the fold cannot rely on insertion order or on one posting per bucket.
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-03-15",
		posting(checking.ID, -4000, 2, usdID),
		posting(groceries.ID, 4000, 2, usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-01-10",
		posting(checking.ID, 250000, 2, usdID),
		posting(salary.ID, -250000, 2, usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-03-15",
		posting(card.ID, -7500, 2, usdID),
		posting(groceries.ID, 7500, 2, usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-02-01",
		posting(checking.ID, -12000, 2, usdID),
		posting(groceries.ID, 12000, 2, usdID),
	), http.StatusCreated)

	for _, bucket := range []string{"day", "week", "month", "quarter", "year"} {
		series := readNetWorthSeriesForSession(t, handler, sessionCookie,
			"?start_date=2026-01-01&end_date=2026-04-30&bucket="+bucket)
		require.NotEmpty(t, series.Buckets, "bucket=%s", bucket)

		for _, seriesBucket := range series.Buckets {
			pointInTime := readNetWorthForSession(t, handler, sessionCookie,
				"?as_of="+seriesBucket.EndDate+"&status=posted")
			assert.Equal(t, pointInTime.Totals, seriesBucket.Totals,
				"bucket=%s end=%s", bucket, seriesBucket.EndDate)
		}
	}
}

// The same equivalence must hold with filters applied, because the account
// subtree expansion is effective-dated and is re-resolved per bucket inside the
// fold. The point-in-time endpoint takes no filters, so each fine bucket is
// checked against a single-bucket series ending on the same date — a request
// that reaches the same fold with one iteration instead of thirty.
func TestNetWorthSeriesFoldKeepsFiltersPerBucket(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	fixture := newSpendingFixture(t, handler)

	filters := "&include_descendants=true&account_id=" + strconvFormatInt(fixture.cashParent.ID) +
		"&commodity_id=" + strconvFormatInt(fixture.usdID)
	daily := readNetWorthSeriesForSession(t, handler, fixture.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&bucket=day"+filters)
	require.Len(t, daily.Buckets, 30)

	for _, bucket := range daily.Buckets {
		single := readNetWorthSeriesForSession(t, handler, fixture.sessionCookie,
			"?start_date=2026-06-01&end_date="+bucket.EndDate+"&bucket=year"+filters)
		require.Len(t, single.Buckets, 1, "end=%s", bucket.EndDate)
		assert.Equal(t, single.Buckets[0].Totals, bucket.Totals, "end=%s", bucket.EndDate)
	}

	// The filter is real: the final balance is the checking subtree in USD only.
	assertBalance(t, daily.Buckets[29].Totals, fixture.usdID, 185000, 2, 185000)
}

// Parent links are effective-dated, so the subtree a filter expands to differs
// per bucket. The fold reads account versions once and replays them forward
// instead of issuing a snapshot query per bucket, so this pins the replay
// against a reparenting that lands mid-series: buckets before the effective
// date must not see the moved subtree, buckets on or after it must.
func TestNetWorthSeriesResolvesSubtreeAgainstEachBucketsAccountVersions(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, usdID := setupAccountAPITest(t, handler)

	grandparent := createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Holdings",
		"account_class":"asset",
		"account_kind":"other_asset",
		"allows_postings":false,
		"opened_on":"2026-01-01",
		"effective_from":"2026-01-01"
	}`)
	parent := createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Cash Group",
		"account_class":"asset",
		"account_kind":"other_asset",
		"allows_postings":false,
		"opened_on":"2026-01-01",
		"effective_from":"2026-01-01"
	}`)
	checking := createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Moved Checking",
		"account_class":"asset",
		"account_kind":"checking",
		"parent_account_id":`+strconvFormatInt(parent.ID)+`,
		"default_commodity_id":`+strconvFormatInt(usdID)+`,
		"quantity_scale_override":2,
		"allows_postings":true,
		"opened_on":"2026-01-01",
		"effective_from":"2026-01-01"
	}`)
	salary := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Moved Salary","category_type":"income"}`)

	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-05",
		posting(checking.ID, 100000, 2, usdID),
		posting(salary.ID, -100000, 2, usdID),
	), http.StatusCreated)

	// The group holds no postings of its own, so it can still be reparented, and
	// the move is backdated into the middle of the series.
	patchAccount(t, handler, sessionCookie, csrfToken, parent.ID, `{
		"name":"Cash Group",
		"account_class":"asset",
		"account_kind":"other_asset",
		"parent_account_id":`+strconvFormatInt(grandparent.ID)+`,
		"allows_postings":false,
		"effective_from":"2026-06-20",
		"change_reason":"Moved cash group under holdings"
	}`, http.StatusOK)

	series := readNetWorthSeriesForSession(t, handler, sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&bucket=day&include_descendants=true&account_id="+strconvFormatInt(grandparent.ID))
	require.Len(t, series.Buckets, 30)

	for _, bucket := range series.Buckets {
		if bucket.EndDate < "2026-06-20" {
			assert.Empty(t, bucket.Totals,
				"before the reparenting the group is not in the holdings subtree (end=%s)", bucket.EndDate)
			continue
		}
		assertBalance(t, bucket.Totals, usdID, 100000, 2, 100000)
	}
}

// TestSpendingAccountFilterMatchesPerJournalEntry pins the tightening that lets
// a cashflow row drill down into this report without the two disagreeing.
//
// The filter keeps category postings whose *journal entry* also touches one of
// the given accounts. Correlating to the transaction instead would count a
// groceries posting that shares only a transaction — not an entry — with the
// filtered account, which is not money spent from that account.
func TestSpendingAccountFilterMatchesPerJournalEntry(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)

	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Checking", "asset", "checking", commodityID, 2)
	savings := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Savings", "asset", "savings", commodityID, 2)
	groceries := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Groceries","category_type":"expense"}`)
	fuel := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Fuel","category_type":"expense"}`)

	// One transaction, two entries: groceries paid from savings, fuel paid from
	// checking. Only the fuel entry touches checking.
	createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-10",
		"journal_entries":[{
			"entry_date":"2026-06-10",
			"postings":[`+posting(savings.ID, -4000, 2, commodityID)+`,`+posting(groceries.ID, 4000, 2, commodityID)+`]
		},{
			"entry_date":"2026-06-10",
			"postings":[`+posting(checking.ID, -6000, 2, commodityID)+`,`+posting(fuel.ID, 6000, 2, commodityID)+`]
		}]
	}`, http.StatusCreated)

	filtered := readSpendingForSession(t, handler, sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&group_by=category&account_id="+strconvFormatInt(checking.ID))

	// Only the fuel entry spent from checking. Matching per transaction would
	// have dragged the groceries entry in with it.
	require.Len(t, filtered.Groups, 1)
	require.NotNil(t, filtered.Groups[0].CategoryID)
	assert.Equal(t, fuel.ID, *filtered.Groups[0].CategoryID)

	// Unfiltered, both entries are still reported: the tightening narrows the
	// account filter, it does not drop postings from the basis.
	unfiltered := readSpendingForSession(t, handler, sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&group_by=category")
	assert.Len(t, unfiltered.Groups, 2)
}

// TestSpendingAndNetWorthOverflowAreA422 covers the other two reports for the
// overflow row of the validation matrix. Both aggregate into the same exact
// accumulator, so both must surface the same stable code rather than a 500 — a
// client cannot retry its way out of exceeding the 38-digit limit.
func TestSpendingAndNetWorthOverflowAreA422(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	fixture := newSpendingFixture(t, handler)

	// Each posting is legal on its own — 38 digits is the limit, not an error —
	// but the report has to sum two of them.
	huge := strings.Repeat("9", 38)
	hugePosting := func(accountID int64, sign string) string {
		return `{"account_id":` + strconvFormatInt(accountID) +
			`,"quantity_value":"` + sign + huge + `","quantity_scale":2,"commodity_id":` +
			strconvFormatInt(fixture.usdID) + `}`
	}
	for _, date := range []string{"2026-06-05", "2026-06-06"} {
		createTransactionForSession(t, handler, fixture.sessionCookie, fixture.csrfToken, balancedBody(date,
			hugePosting(fixture.checking.ID, "-"),
			hugePosting(fixture.groceries.ID, ""),
		), http.StatusCreated)
	}

	for name, target := range map[string]string{
		"spending":  "/api/v1/reports/spending?start_date=2026-06-01&end_date=2026-06-30&group_by=category",
		"net worth": "/api/v1/reports/net-worth?start_date=2026-06-01&end_date=2026-06-30&bucket=month",
	} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.AddCookie(fixture.sessionCookie)
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		require.Equal(t, http.StatusUnprocessableEntity, res.Code, "%s: %s", name, res.Body.String())
		var body errorResponse
		require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
		assert.Equal(t, "LEDGER_OVERFLOW", body.Error.Code, name)
	}
}

// TestSpendingRanksSubUnitAmountsByValue pins ranking below one whole unit.
// The comparison used to truncate each total to whole units first, so every
// group under 1.00 ranked as zero and fell through to the identity tiebreak —
// the case any instrument or crypto quantity lands in, and one an account
// created earlier would silently win.
func TestSpendingRanksSubUnitAmountsByValue(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	fixture := newSpendingFixture(t, handler)

	// Groceries is the older account, so the identity tiebreak favours it. Give
	// it the smaller amount: only a value comparison puts travel first.
	// October: the fixture seeds June and July, and this ranking must be read
	// against only the two amounts under test.
	createTransactionForSession(t, handler, fixture.sessionCookie, fixture.csrfToken, balancedBody("2026-10-05",
		posting(fixture.checking.ID, -10, 2, fixture.usdID),
		posting(fixture.groceries.ID, 10, 2, fixture.usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, fixture.sessionCookie, fixture.csrfToken, balancedBody("2026-10-06",
		posting(fixture.checking.ID, -90, 2, fixture.usdID),
		posting(fixture.travel.ID, 90, 2, fixture.usdID),
	), http.StatusCreated)

	require.Less(t, fixture.groceries.ID, fixture.travel.ID)

	report := readSpendingForSession(t, handler, fixture.sessionCookie,
		"?start_date=2026-10-01&end_date=2026-10-31&group_by=category")

	require.Len(t, report.Groups, 2)
	assert.Equal(t, fixture.travel.ID, *report.Groups[0].CategoryID, "0.90 must outrank 0.10")
	assertGroupTotal(t, report.Groups[0], fixture.usdID, "90", 2)
	assert.Equal(t, fixture.groceries.ID, *report.Groups[1].CategoryID)
	assertGroupTotal(t, report.Groups[1], fixture.usdID, "10", 2)
}

// TestSpendingRanksAcrossScalesByValueNotDigitCount is the other half of the
// same rule: an exact comparison must still not let a deep scale win on digit
// count alone. 1.90 outranks 1.10 even though both are "1" in whole units, and
// a scale-8 quantity worth less than a scale-2 one still ranks below it.
func TestSpendingRanksAcrossScalesByValueNotDigitCount(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	fixture := newSpendingFixture(t, handler)

	createTransactionForSession(t, handler, fixture.sessionCookie, fixture.csrfToken, balancedBody("2026-09-05",
		posting(fixture.checking.ID, -110, 2, fixture.usdID),
		posting(fixture.groceries.ID, 110, 2, fixture.usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, fixture.sessionCookie, fixture.csrfToken, balancedBody("2026-09-06",
		posting(fixture.checking.ID, -190, 2, fixture.usdID),
		posting(fixture.travel.ID, 190, 2, fixture.usdID),
	), http.StatusCreated)

	report := readSpendingForSession(t, handler, fixture.sessionCookie,
		"?start_date=2026-09-01&end_date=2026-09-30&group_by=category")

	require.Len(t, report.Groups, 2)
	assert.Equal(t, fixture.travel.ID, *report.Groups[0].CategoryID, "1.90 must outrank 1.10")
	assert.Equal(t, fixture.groceries.ID, *report.Groups[1].CategoryID)
}

// TestReportAccountFilterRejectsAccountsThatHoldNoReadableBalance pins the
// shared account-filter contract. The filter always means "the account the
// money sat in or moved through", which only asset and liability accounts
// answer. An equity or category account used to be accepted and then silently
// skipped, so the report came back empty with nothing to explain it.
func TestReportAccountFilterRejectsAccountsThatHoldNoReadableBalance(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	fixture := newSpendingFixture(t, handler)

	// commodity_trading is the reachable equity account: users cannot create one
	// ("equity accounts are managed by system workflows") and the picker never
	// offers a system account, so this arrives only by hand-editing a URL — which
	// is exactly what a shareable-URL report invites.
	systemAccounts := listAccountsForSession(t, handler, fixture.sessionCookie, "?include_system=true")
	equity := accountBySystemRole(t, systemAccounts.Accounts, "commodity_trading")

	for name, target := range map[string]string{
		"net worth": "/api/v1/reports/net-worth?start_date=2026-06-01&end_date=2026-06-30&bucket=month&account_id=",
		"spending":  "/api/v1/reports/spending?start_date=2026-06-01&end_date=2026-06-30&group_by=category&account_id=",
		"cashflow":  "/api/v1/reports/cashflow?start_date=2026-06-01&end_date=2026-06-30&bucket=month&account_id=",
	} {
		req := httptest.NewRequest(http.MethodGet, target+strconvFormatInt(equity.ID), nil)
		req.AddCookie(fixture.sessionCookie)
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		require.Equal(t, http.StatusBadRequest, res.Code, "%s: %s", name, res.Body.String())
		var body errorResponse
		require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
		assert.Equal(t, "VALIDATION_FAILED", body.Error.Code, name)
	}

	// A category account is the other reachable case: the picker excludes them,
	// but a hand-edited or stale link can still name one.
	for name, target := range map[string]string{
		"net worth": "/api/v1/reports/net-worth?start_date=2026-06-01&end_date=2026-06-30&bucket=month&account_id=",
		"spending":  "/api/v1/reports/spending?start_date=2026-06-01&end_date=2026-06-30&group_by=category&account_id=",
	} {
		req := httptest.NewRequest(http.MethodGet, target+strconvFormatInt(fixture.groceries.ID), nil)
		req.AddCookie(fixture.sessionCookie)
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		assert.Equal(t, http.StatusBadRequest, res.Code, "%s: %s", name, res.Body.String())
	}

	// The liability account is still a legal filter on every report: it holds a
	// balance, so the question has an answer.
	for name, target := range map[string]string{
		"net worth": "/api/v1/reports/net-worth?start_date=2026-06-01&end_date=2026-06-30&bucket=month&account_id=",
		"spending":  "/api/v1/reports/spending?start_date=2026-06-01&end_date=2026-06-30&group_by=category&account_id=",
	} {
		req := httptest.NewRequest(http.MethodGet, target+strconvFormatInt(fixture.card.ID), nil)
		req.AddCookie(fixture.sessionCookie)
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		assert.Equal(t, http.StatusOK, res.Code, "%s: %s", name, res.Body.String())
	}
}

// TestSpendingCategoryFilterRejectsAccountsThatAreNotCategories closes the
// other half. Categories are income and expense accounts, so every account id
// is a syntactically valid category id and an existence check alone let a
// checking account through — answering with an empty report rather than saying
// the question was malformed.
func TestSpendingCategoryFilterRejectsAccountsThatAreNotCategories(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	fixture := newSpendingFixture(t, handler)

	for name, categoryID := range map[string]int64{
		"a checking account": fixture.checking.ID,
		"a credit card":      fixture.card.ID,
	} {
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/reports/spending?start_date=2026-06-01&end_date=2026-06-30&group_by=category&category_id="+
				strconvFormatInt(categoryID), nil)
		req.AddCookie(fixture.sessionCookie)
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		require.Equal(t, http.StatusBadRequest, res.Code, "%s: %s", name, res.Body.String())
		var body errorResponse
		require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
		assert.Equal(t, "VALIDATION_FAILED", body.Error.Code, name)
	}

	// A real category still narrows the report rather than being rejected.
	report := readSpendingForSession(t, handler, fixture.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&group_by=category&category_id="+
			strconvFormatInt(fixture.groceries.ID))
	require.Len(t, report.Groups, 1)
	assert.Equal(t, fixture.groceries.ID, *report.Groups[0].CategoryID)
}

// TestSpendingRankingNeverComparesUnlikeCommodities pins the ordering policy.
// A group whose only total is in another commodity cannot be placed by value
// against groups measured in the ranking commodity, so it sorts after them
// rather than jumping the queue on a number that was never valued — 900.00 EUR
// is not "more than" 10.00 USD until a reporting currency says so.
func TestSpendingRankingNeverComparesUnlikeCommodities(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	fixture := newSpendingFixture(t, handler)

	dining := createCategoryForSession(t, handler, fixture.sessionCookie, fixture.csrfToken,
		`{"name":"Dining","category_type":"expense"}`)

	// Two groups in USD, one in EUR only. USD ranks the report because it is the
	// commodity present in the most groups.
	createTransactionForSession(t, handler, fixture.sessionCookie, fixture.csrfToken, balancedBody("2026-11-05",
		posting(fixture.checking.ID, -1000, 2, fixture.usdID),
		posting(fixture.groceries.ID, 1000, 2, fixture.usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, fixture.sessionCookie, fixture.csrfToken, balancedBody("2026-11-06",
		posting(fixture.checking.ID, -500, 2, fixture.usdID),
		posting(fixture.travel.ID, 500, 2, fixture.usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, fixture.sessionCookie, fixture.csrfToken, balancedBody("2026-11-07",
		posting(fixture.euroCash.ID, -90000, 2, fixture.eurID),
		posting(dining.ID, 90000, 2, fixture.eurID),
	), http.StatusCreated)

	report := readSpendingForSession(t, handler, fixture.sessionCookie,
		"?start_date=2026-11-01&end_date=2026-11-30&group_by=category")

	require.Len(t, report.Groups, 3)
	// 10.00 USD then 5.00 USD: a real comparison, both in one commodity.
	assert.Equal(t, fixture.groceries.ID, *report.Groups[0].CategoryID)
	assert.Equal(t, fixture.travel.ID, *report.Groups[1].CategoryID)
	// The EUR-only group last, because its magnitude says nothing about the
	// others. A "largest magnitude wins" rule would have ranked it first.
	assert.Equal(t, dining.ID, *report.Groups[2].CategoryID)
}
