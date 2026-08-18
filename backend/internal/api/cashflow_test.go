package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
