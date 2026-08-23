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

type cashflowFixture struct {
	sessionCookie *http.Cookie
	csrfToken     string
	usdID         int64
	checking      accountResponse
	savings       accountResponse
	card          accountResponse
	groceries     categoryResponse
	salary        categoryResponse
}

// newCashflowFixture builds the liquid-cash shape the cashflow report is
// defined over: two accounts inside the default scope, one liability outside
// it, and income and expense categories to move against.
func newCashflowFixture(t *testing.T, handler http.Handler) cashflowFixture {
	t.Helper()

	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")
	currencySetup := completeCurrencySetupForSession(t, handler, sessionCookie, csrfToken, "USD", []setupCurrencySelectionRequest{
		{Code: "USD", Name: "US Dollar"},
	})
	usdID := currencySetup.DefaultCurrency.ID
	mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/setup/system-accounts", http.StatusCreated)

	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Checking", "asset", "checking", usdID, 2)
	savings := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Savings", "asset", "savings", usdID, 2)
	card := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Credit Card", "liability", "credit_card", usdID, 2)

	groceries := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Groceries","category_type":"expense"}`)
	salary := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Salary","category_type":"income"}`)

	return cashflowFixture{
		sessionCookie: sessionCookie,
		csrfToken:     csrfToken,
		usdID:         usdID,
		checking:      checking,
		savings:       savings,
		card:          card,
		groceries:     groceries,
		salary:        salary,
	}
}

func TestCashflowClassifiesInflowOutflowAndTransfers(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newCashflowFixture(t, handler)

	// 2000.00 salary in.
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-01",
		posting(f.checking.ID, 200000, 2, f.usdID),
		posting(f.salary.ID, -200000, 2, f.usdID),
	), http.StatusCreated)
	// 120.00 groceries out.
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-03",
		posting(f.checking.ID, -12000, 2, f.usdID),
		posting(f.groceries.ID, 12000, 2, f.usdID),
	), http.StatusCreated)
	// 300.00 checking to savings: both are inside the default scope, so this
	// changes neither the cash total nor the user's cashflow and must vanish.
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-05",
		posting(f.checking.ID, -30000, 2, f.usdID),
		posting(f.savings.ID, 30000, 2, f.usdID),
	), http.StatusCreated)
	// 50.00 card payment: the card is outside the scope, so this is financing
	// movement out — never spending.
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-07",
		posting(f.checking.ID, -5000, 2, f.usdID),
		posting(f.card.ID, 5000, 2, f.usdID),
	), http.StatusCreated)
	// 1000.00 drawn on the card into checking: financing movement in.
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-09",
		posting(f.checking.ID, 100000, 2, f.usdID),
		posting(f.card.ID, -100000, 2, f.usdID),
	), http.StatusCreated)
	// A voided expense is not financial truth.
	voided := createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-11",
		posting(f.checking.ID, -9900, 2, f.usdID),
		posting(f.groceries.ID, 9900, 2, f.usdID),
	), http.StatusCreated)
	mutateTransaction(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/transactions/"+strconvFormatInt(voided.ID)+"/void",
		`{"change_reason":"cashflow fixture"}`, http.StatusOK)
	// Outside the range.
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-07-02",
		posting(f.checking.ID, -7700, 2, f.usdID),
		posting(f.groceries.ID, 7700, 2, f.usdID),
	), http.StatusCreated)

	report := readCashflowForSession(t, handler, f.sessionCookie, "?start_date=2026-06-01&end_date=2026-06-30&bucket=month")
	require.Len(t, report.Buckets, 1)
	bucket := report.Buckets[0]

	assertBalance(t, bucket.Inflow, f.usdID, 200000, 2, 200000)
	assertBalance(t, bucket.Outflow, f.usdID, 12000, 2, 12000)
	assertBalance(t, bucket.TransferIn, f.usdID, 100000, 2, 100000)
	assertBalance(t, bucket.TransferOut, f.usdID, 5000, 2, 5000)
	// 2000.00 - 120.00
	assertBalance(t, bucket.OperatingNet, f.usdID, 188000, 2, 188000)
	// 1000.00 - 50.00
	assertBalance(t, bucket.TransferNet, f.usdID, 95000, 2, 95000)
	// 2000 - 120 - 50 + 1000; the internal 300.00 transfer cancels.
	assertBalance(t, bucket.NetMovement, f.usdID, 283000, 2, 283000)

	assert.Equal(t, "default_liquid_cash", report.Query.CashScope)
	assert.Contains(t, report.Query.CashKinds, "checking")
	assert.Contains(t, report.Query.CashKinds, "savings")
	// The default scope is named, not invisible: it says which accounts it came
	// out as.
	assert.ElementsMatch(t, []int64{f.checking.ID, f.savings.ID}, report.Query.Filters.ResolvedAccountIDs)
}

func TestCashflowClassifiesEveryCounterpartOfOneEntry(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newCashflowFixture(t, handler)

	// One entry, three counterparts: salary in, groceries out, and part of the
	// pay withheld against the card. A "first counterpart wins" classification
	// would report this as pure income; each posting has to be classified on
	// its own account class.
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-04",
		posting(f.checking.ID, 150000, 2, f.usdID),
		posting(f.salary.ID, -200000, 2, f.usdID),
		posting(f.groceries.ID, 20000, 2, f.usdID),
		posting(f.card.ID, 30000, 2, f.usdID),
	), http.StatusCreated)

	report := readCashflowForSession(t, handler, f.sessionCookie, "?start_date=2026-06-01&end_date=2026-06-30&bucket=month")
	require.Len(t, report.Buckets, 1)
	bucket := report.Buckets[0]

	assertBalance(t, bucket.Inflow, f.usdID, 200000, 2, 200000)
	assertBalance(t, bucket.Outflow, f.usdID, 20000, 2, 20000)
	assertBalance(t, bucket.TransferOut, f.usdID, 30000, 2, 30000)
	assertBalance(t, bucket.OperatingNet, f.usdID, 180000, 2, 180000)
	assertBalance(t, bucket.TransferNet, f.usdID, -30000, 2, -30000)
	// The cash leg itself: 2000 - 200 - 300.
	assertBalance(t, bucket.NetMovement, f.usdID, 150000, 2, 150000)
}

func TestCashflowNetMovementReconcilesInEveryBucket(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newCashflowFixture(t, handler)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-02",
		posting(f.checking.ID, 90000, 2, f.usdID),
		posting(f.salary.ID, -90000, 2, f.usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-07-06",
		posting(f.checking.ID, -4500, 2, f.usdID),
		posting(f.groceries.ID, 4500, 2, f.usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-07-20",
		posting(f.savings.ID, -1000, 2, f.usdID),
		posting(f.card.ID, 1000, 2, f.usdID),
	), http.StatusCreated)

	report := readCashflowForSession(t, handler, f.sessionCookie, "?start_date=2026-06-01&end_date=2026-07-31&bucket=month")
	require.Len(t, report.Buckets, 2)

	// The identity that makes the report auditable, asserted per bucket rather
	// than only on the totals.
	for _, bucket := range report.Buckets {
		assertCashflowIdentity(t, bucket, f.usdID)
	}

	assertBalance(t, report.Buckets[0].NetMovement, f.usdID, 90000, 2, 90000)
	// July: -45.00 groceries and -10.00 out to the card.
	assertBalance(t, report.Buckets[1].NetMovement, f.usdID, -5500, 2, -5500)

	// And it reconciles to the cash accounts' own balance change: net worth over
	// the same accounts moves by the same amount.
	netWorth := readNetWorthSeriesForSession(t, handler, f.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-07-31&bucket=month"+
			"&account_id="+strconvFormatInt(f.checking.ID)+"&account_id="+strconvFormatInt(f.savings.ID))
	require.Len(t, netWorth.Buckets, 2)
	assertBalance(t, netWorth.Buckets[0].Totals, f.usdID, 90000, 2, 90000)
	// End of July = 900.00 + (-55.00).
	assertBalance(t, netWorth.Buckets[1].Totals, f.usdID, 84500, 2, 84500)
}

func TestCashflowReturnsEmptyBucketsRatherThanOmittingPeriods(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newCashflowFixture(t, handler)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-02",
		posting(f.checking.ID, 50000, 2, f.usdID),
		posting(f.salary.ID, -50000, 2, f.usdID),
	), http.StatusCreated)

	report := readCashflowForSession(t, handler, f.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-08-31&bucket=month")

	// A month with no activity is still a row: a gap in a cashflow table reads
	// as missing data rather than as a quiet month, and every bucket in the
	// range must reconcile via the same identity, including the empty ones.
	require.Len(t, report.Buckets, 3)
	assert.Empty(t, report.Buckets[1].Inflow)
	assert.Empty(t, report.Buckets[1].NetMovement)
	assert.Empty(t, report.Buckets[2].Inflow)
	assert.Empty(t, report.Buckets[2].NetMovement)
	for _, bucket := range report.Buckets {
		assertCashflowIdentity(t, bucket, f.usdID)
	}
}

func TestCashflowSelectedAccountsChangeWhatCountsAsTransfer(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newCashflowFixture(t, handler)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-05",
		posting(f.checking.ID, -30000, 2, f.usdID),
		posting(f.savings.ID, 30000, 2, f.usdID),
	), http.StatusCreated)

	// Both accounts selected: the movement is internal. It nets to an explicit
	// zero rather than vanishing — the commodity did move, and the report says
	// the move left the cash total unchanged — and it is classified nowhere.
	both := readCashflowForSession(t, handler, f.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&bucket=month"+
			"&account_id="+strconvFormatInt(f.checking.ID)+"&account_id="+strconvFormatInt(f.savings.ID))
	require.Len(t, both.Buckets, 1)
	assertBalance(t, both.Buckets[0].NetMovement, f.usdID, 0, 2, 0)
	assert.Empty(t, both.Buckets[0].TransferOut)
	assert.Empty(t, both.Buckets[0].Outflow)
	assert.Empty(t, both.Buckets[0].Inflow)
	assert.Equal(t, "selected_accounts", both.Query.CashScope)

	// Only checking selected: the same movement is now financing movement out,
	// and never becomes spending.
	checkingOnly := readCashflowForSession(t, handler, f.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&bucket=month&account_id="+strconvFormatInt(f.checking.ID))
	require.Len(t, checkingOnly.Buckets, 1)
	assertBalance(t, checkingOnly.Buckets[0].TransferOut, f.usdID, 30000, 2, 30000)
	assertBalance(t, checkingOnly.Buckets[0].NetMovement, f.usdID, -30000, 2, -30000)
	assert.Empty(t, checkingOnly.Buckets[0].Outflow)
}

func TestCashflowRejectsInvalidQuery(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newCashflowFixture(t, handler)

	for name, suffix := range map[string]string{
		"missing bucket":       "?start_date=2026-06-01&end_date=2026-06-30",
		"invalid bucket":       "?start_date=2026-06-01&end_date=2026-06-30&bucket=fortnight",
		"inverted range":       "?start_date=2026-06-30&end_date=2026-06-01&bucket=month",
		"inaccessible account": "?start_date=2026-06-01&end_date=2026-06-30&bucket=month&account_id=999999",
		"unknown commodity":    "?start_date=2026-06-01&end_date=2026-06-30&bucket=month&commodity_id=999999",
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/cashflow"+suffix, nil)
		req.AddCookie(f.sessionCookie)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		assert.Equal(t, http.StatusBadRequest, res.Code, name)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/cashflow?start_date=2026-06-01&end_date=2026-06-30&bucket=month", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	assert.Equal(t, http.StatusUnauthorized, res.Code)
}

func assertCashflowIdentity(t *testing.T, bucket cashflowBucketResponse, commodityID int64) {
	t.Helper()

	value := func(balances []balanceQuantityResponse) string {
		for _, balance := range balances {
			if balance.CommodityID == commodityID {
				return balance.QuantityValue.String()
			}
		}
		return "0"
	}

	operating := value(bucket.OperatingNet)
	transfer := value(bucket.TransferNet)
	net := value(bucket.NetMovement)

	operatingInt := mustParseInt64(t, operating)
	transferInt := mustParseInt64(t, transfer)
	netInt := mustParseInt64(t, net)
	assert.Equal(t, netInt, operatingInt+transferInt,
		"net_movement must equal operating_net + transfer_net in bucket %s", bucket.StartDate)
}

func readCashflowForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, suffix string) cashflowResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/cashflow"+suffix, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	var response cashflowResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}

// requestCashflowForSession is readCashflowForSession without the success
// assertion, for the cases whose subject is the rejection.
// quantityFor picks one commodity's entry out of a measure.
func quantityFor(t *testing.T, quantities []balanceQuantityResponse, commodityID int64) balanceQuantityResponse {
	t.Helper()

	for _, quantity := range quantities {
		if quantity.CommodityID == commodityID {
			return quantity
		}
	}
	require.Failf(t, "commodity not present in measure", "commodity %d", commodityID)
	return balanceQuantityResponse{}
}

func requestCashflowForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, suffix string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/cashflow"+suffix, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)
	return res
}

func apiErrorCode(t *testing.T, res *httptest.ResponseRecorder) string {
	t.Helper()

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	return body.Error.Code
}

// TestCashflowRejectsSystemAccountsInTheCashScope pins the one promise this
// endpoint makes about its scope in every response it sends. Selecting a system
// account explicitly used to be accepted, which produced a report that summed
// transfer clearing while its own excluded_system_roles said ["all"].
func TestCashflowRejectsSystemAccountsInTheCashScope(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newCashflowFixture(t, handler)

	systemAccounts := listAccountsForSession(t, handler, f.sessionCookie, "?include_system=true")
	transferClearing := accountBySystemRole(t, systemAccounts.Accounts, "transfer_clearing")
	commodityTrading := accountBySystemRole(t, systemAccounts.Accounts, "commodity_trading")

	// transfer_clearing is an asset account, so a class check alone would let it
	// through: the system role is what disqualifies it.
	for name, accountID := range map[string]int64{
		"transfer clearing": transferClearing.ID,
		"commodity trading": commodityTrading.ID,
	} {
		res := requestCashflowForSession(t, handler, f.sessionCookie,
			"?start_date=2026-06-01&end_date=2026-06-30&bucket=month&account_id="+strconvFormatInt(accountID))
		require.Equal(t, http.StatusBadRequest, res.Code, name)
		assert.Equal(t, "VALIDATION_FAILED", apiErrorCode(t, res), name)
	}
}

// TestCashflowScopeTakesAssetAndLiabilityAccountsOnly records the other half of
// the scope policy: a credit card is a legal cash scope because it has a balance
// net_movement reconciles to, while a category account has none.
func TestCashflowScopeTakesAssetAndLiabilityAccountsOnly(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newCashflowFixture(t, handler)

	// 40.00 of groceries on the card: the card owes more, and the card-scoped
	// report reads that as outflow, not as an unclassifiable posting.
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-05",
		posting(f.card.ID, -4000, 2, f.usdID),
		posting(f.groceries.ID, 4000, 2, f.usdID),
	), http.StatusCreated)

	card := readCashflowForSession(t, handler, f.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&bucket=month&account_id="+strconvFormatInt(f.card.ID))
	require.Len(t, card.Buckets, 1)
	assert.Equal(t, "selected_accounts", card.Query.CashScope)
	assertBalance(t, card.Buckets[0].Outflow, f.usdID, 4000, 2, 4000)
	assertBalance(t, card.Buckets[0].NetMovement, f.usdID, -4000, 2, -4000)

	// A category is an expense account: it holds no balance to move, so naming
	// one as "cash" is a malformed question rather than an empty report.
	res := requestCashflowForSession(t, handler, f.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&bucket=month&account_id="+strconvFormatInt(f.groceries.ID))
	require.Equal(t, http.StatusBadRequest, res.Code)
	assert.Equal(t, "VALIDATION_FAILED", apiErrorCode(t, res))
}

// TestCashflowOverflowIsA422RatherThanA500 pins the error the validation matrix
// names for exact arithmetic: an aggregate wider than the 38-digit coefficient
// limit is a stable LEDGER_OVERFLOW the client can explain, not an internal
// error it should offer to retry.
func TestCashflowOverflowIsA422RatherThanA500(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newCashflowFixture(t, handler)

	// Two postings that are each legal on their own — 38 digits is the limit,
	// not an error — but whose sum needs 39.
	huge := strings.Repeat("9", 38)
	hugePosting := func(accountID int64, sign string) string {
		return `{"account_id":` + strconvFormatInt(accountID) +
			`,"quantity_value":"` + sign + huge + `","quantity_scale":2,"commodity_id":` +
			strconvFormatInt(f.usdID) + `}`
	}
	for _, date := range []string{"2026-06-05", "2026-06-06"} {
		createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody(date,
			hugePosting(f.checking.ID, "-"),
			hugePosting(f.groceries.ID, ""),
		), http.StatusCreated)
	}

	res := requestCashflowForSession(t, handler, f.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&bucket=month")
	require.Equal(t, http.StatusUnprocessableEntity, res.Code, res.Body.String())
	assert.Equal(t, "LEDGER_OVERFLOW", apiErrorCode(t, res))
}

// TestCashflowKeepsEachMeasureAtTheScaleItAccumulated pins what the response
// promises about scales: each measure is paired with its own, and none is
// restated to match a sibling. A posting may be recorded at any scale its
// commodity permits, and the seven measures accumulate independently, so 50.00
// of income and 100 of expense really do come back as scale 2 and scale 0 in
// the same commodity and bucket. Pairing them correctly is the client's job —
// see the frontend's per-measure scales — and rewriting the numbers to make
// that job easier is what TestCashflowDoesNotManufactureOverflow... rules out.
func TestCashflowKeepsEachMeasureAtTheScaleItAccumulated(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newCashflowFixture(t, handler)

	// 50.00 salary in at scale 2, 100 groceries out at scale 0.
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-05",
		posting(f.checking.ID, 5000, 2, f.usdID),
		posting(f.salary.ID, -5000, 2, f.usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-06",
		posting(f.checking.ID, -100, 0, f.usdID),
		posting(f.groceries.ID, 100, 0, f.usdID),
	), http.StatusCreated)

	report := readCashflowForSession(t, handler, f.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&bucket=month&account_id="+strconvFormatInt(f.checking.ID))
	require.Len(t, report.Buckets, 1)
	bucket := report.Buckets[0]

	inflow := quantityFor(t, bucket.Inflow, f.usdID)
	assert.Equal(t, "5000", inflow.QuantityValue.String())
	assert.Equal(t, 2, inflow.QuantityScale)

	outflow := quantityFor(t, bucket.Outflow, f.usdID)
	assert.Equal(t, "100", outflow.QuantityValue.String())
	assert.Equal(t, 0, outflow.QuantityScale)

	// The derived measures combine the two, so they carry the deeper scale the
	// combination needs: 50.00 - 100.00 = -50.00.
	assertBalance(t, bucket.OperatingNet, f.usdID, -5000, 2, -5000)
	assertBalance(t, bucket.NetMovement, f.usdID, -5000, 2, -5000)
}

// TestCashflowDoesNotManufactureOverflowFromAnUnrelatedMeasure is the boundary
// on writing a commodity's measures at one scale. Restating a coefficient at a
// deeper scale costs a digit per decimal place, and the coefficient ceiling is
// 38 digits — so a measure that is perfectly representable at the scale it was
// recorded at must not be pushed past the limit because some *other* measure of
// the same commodity happened to carry more precision.
func TestCashflowDoesNotManufactureOverflowFromAnUnrelatedMeasure(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newCashflowFixture(t, handler)

	// A 38-digit inflow at scale 0 — the largest coefficient the ledger accepts —
	// and an equal outflow, so operating_net is zero and net_movement stays
	// tiny. Every individual measure is representable exactly as recorded.
	huge := strings.Repeat("9", 38)
	scaleZero := func(accountID int64, sign string) string {
		return `{"account_id":` + strconvFormatInt(accountID) +
			`,"quantity_value":"` + sign + huge + `","quantity_scale":0,"commodity_id":` +
			strconvFormatInt(f.usdID) + `}`
	}
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-05",
		scaleZero(f.checking.ID, ""),
		scaleZero(f.salary.ID, "-"),
	), http.StatusCreated)
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-06",
		scaleZero(f.checking.ID, "-"),
		scaleZero(f.groceries.ID, ""),
	), http.StatusCreated)

	// A 2.50 transfer out to an account outside the scope. It is tiny, but it
	// makes scale 2 the deepest precision this commodity carries in the bucket.
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-07",
		posting(f.checking.ID, -250, 2, f.usdID),
		posting(f.savings.ID, 250, 2, f.usdID),
	), http.StatusCreated)

	report := readCashflowForSession(t, handler, f.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&bucket=month&account_id="+strconvFormatInt(f.checking.ID))
	require.Len(t, report.Buckets, 1)

	// Every measure comes back at the scale it was produced at, losslessly.
	inflow := quantityFor(t, report.Buckets[0].Inflow, f.usdID)
	assert.Equal(t, huge, inflow.QuantityValue.String())
	assert.Equal(t, 0, inflow.QuantityScale)

	outflow := quantityFor(t, report.Buckets[0].Outflow, f.usdID)
	assert.Equal(t, huge, outflow.QuantityValue.String())
	assert.Equal(t, 0, outflow.QuantityScale)

	transferOut := quantityFor(t, report.Buckets[0].TransferOut, f.usdID)
	assert.Equal(t, "250", transferOut.QuantityValue.String())
	assert.Equal(t, 2, transferOut.QuantityScale)

	// The results that combine them stay small, so they are representable at the
	// deeper scale the combination needs.
	assertBalance(t, report.Buckets[0].OperatingNet, f.usdID, 0, 0, 0)
	assertBalance(t, report.Buckets[0].NetMovement, f.usdID, -250, 2, -250)
}
