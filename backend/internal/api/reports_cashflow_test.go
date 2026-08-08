package api

import (
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertCashflowReconciles pins the invariant the whole report rests on. It is
// asserted on every scenario below rather than in one dedicated test, because
// the point of deriving classification from double entry is that it holds for
// *every* shape of transaction, not for the ones someone thought to check.
func assertCashflowReconciles(t *testing.T, total cashflowBucketTotalResponse) {
	t.Helper()

	inflow := total.Inflow.BigInt()
	outflow := total.Outflow.BigInt()
	transferIn := total.TransferIn.BigInt()
	transferOut := total.TransferOut.BigInt()

	operating := new(big.Int).Sub(inflow, outflow)
	assert.Equal(t, operating.String(), total.OperatingNet.String(),
		"operating_net must equal inflow - outflow")

	net := new(big.Int).Add(operating, transferIn)
	net.Sub(net, transferOut)
	assert.Equal(t, net.String(), total.NetMovement.String(),
		"net_movement must equal inflow - outflow + transfer_in - transfer_out")
}

func TestCashflowSeparatesOperatingMovementFromTransfers(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, usdID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Flow Checking", "asset", "checking", usdID, 2)
	property := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Flow Property", "asset", "property", usdID, 2)
	salary := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Flow Salary","category_type":"income"}`)
	rent := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Flow Rent","category_type":"expense"}`)

	// 2000.00 in from income, 900.00 out to an expense.
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-02",
		posting(checking.ID, 200000, 2, usdID),
		posting(salary.ID, -200000, 2, usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-05",
		posting(checking.ID, -90000, 2, usdID),
		posting(rent.ID, 90000, 2, usdID),
	), http.StatusCreated)
	// 400.00 out to an asset outside the cash scope: financing movement, not
	// spending.
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -40000, 2, usdID),
		posting(property.ID, 40000, 2, usdID),
	), http.StatusCreated)

	report := readCashflowForSession(t, handler, sessionCookie, "?start_date=2026-06-01&end_date=2026-06-30&bucket=month")

	require.Len(t, report.Buckets, 1)
	require.Len(t, report.Buckets[0].Totals, 1)
	total := report.Buckets[0].Totals[0]

	assert.Equal(t, "200000", total.Inflow.String())
	assert.Equal(t, "90000", total.Outflow.String())
	// Operating net excludes the property purchase entirely.
	assert.Equal(t, "110000", total.OperatingNet.String())
	assert.Equal(t, "0", total.TransferIn.String())
	assert.Equal(t, "40000", total.TransferOut.String())
	assert.Equal(t, "70000", total.NetMovement.String())
	assertCashflowReconciles(t, total)
}

func TestCashflowEliminatesTransfersBetweenTwoInScopeAccounts(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, usdID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Elim Checking", "asset", "checking", usdID, 2)
	savings := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Elim Savings", "asset", "savings", usdID, 2)
	salary := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Elim Salary","category_type":"income"}`)

	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-02",
		posting(checking.ID, 100000, 2, usdID),
		posting(salary.ID, -100000, 2, usdID),
	), http.StatusCreated)
	// Checking to savings: both are in the default cash scope, so this changed
	// neither the cash total nor the user's cashflow.
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-08",
		posting(checking.ID, -60000, 2, usdID),
		posting(savings.ID, 60000, 2, usdID),
	), http.StatusCreated)

	report := readCashflowForSession(t, handler, sessionCookie, "?start_date=2026-06-01&end_date=2026-06-30&bucket=month")

	require.Len(t, report.Buckets, 1)
	total := report.Buckets[0].Totals[0]

	assert.Equal(t, "100000", total.Inflow.String())
	assert.Equal(t, "0", total.Outflow.String())
	// The internal move contributes nothing at all — not to transfers either.
	assert.Equal(t, "0", total.TransferIn.String())
	assert.Equal(t, "0", total.TransferOut.String())
	assert.Equal(t, "100000", total.NetMovement.String())
	assertCashflowReconciles(t, total)
}

func TestCashflowClassifiesEveryCounterpartOfASplitTransaction(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, usdID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Split Checking", "asset", "checking", usdID, 2)
	savings := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Split Savings", "asset", "savings", usdID, 2)
	property := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Split Property", "asset", "property", usdID, 2)
	groceries := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Split Groceries","category_type":"expense"}`)
	fees := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Split Fees","category_type":"expense"}`)

	// One 1000.00 withdrawal funding four different things at once. A
	// "first counterpart wins" rule would classify the whole 1000.00 as
	// whichever leg happened to come first; every leg must count separately.
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-04",
		posting(checking.ID, -100000, 2, usdID),
		posting(groceries.ID, 30000, 2, usdID),
		posting(fees.ID, 10000, 2, usdID),
		posting(savings.ID, 25000, 2, usdID),
		posting(property.ID, 35000, 2, usdID),
	), http.StatusCreated)

	report := readCashflowForSession(t, handler, sessionCookie, "?start_date=2026-06-01&end_date=2026-06-30&bucket=month")

	require.Len(t, report.Buckets, 1)
	total := report.Buckets[0].Totals[0]

	assert.Equal(t, "0", total.Inflow.String())
	// Both expense legs, and only those.
	assert.Equal(t, "40000", total.Outflow.String())
	assert.Equal(t, "-40000", total.OperatingNet.String())
	// Only the property leg leaves the cash scope; the savings leg is internal.
	assert.Equal(t, "0", total.TransferIn.String())
	assert.Equal(t, "35000", total.TransferOut.String())
	// Cash fell by 1000.00 but 250.00 of that landed back in savings, which is
	// also in scope, so the scope's net movement is -750.00.
	assert.Equal(t, "-75000", total.NetMovement.String())
	assertCashflowReconciles(t, total)
}

func TestCashflowNetMovementMatchesTheCashBalanceChange(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, usdID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Recon Checking", "asset", "checking", usdID, 2)
	savings := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Recon Savings", "asset", "savings", usdID, 2)
	property := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Recon Property", "asset", "property", usdID, 2)
	salary := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Recon Salary","category_type":"income"}`)
	groceries := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Recon Groceries","category_type":"expense"}`)

	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-02",
		posting(checking.ID, 300000, 2, usdID),
		posting(salary.ID, -300000, 2, usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-11",
		posting(checking.ID, -45000, 2, usdID),
		posting(groceries.ID, 45000, 2, usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-18",
		posting(checking.ID, -80000, 2, usdID),
		posting(savings.ID, 80000, 2, usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-25",
		posting(savings.ID, -120000, 2, usdID),
		posting(property.ID, 120000, 2, usdID),
	), http.StatusCreated)

	report := readCashflowForSession(t, handler, sessionCookie, "?start_date=2026-06-01&end_date=2026-06-30&bucket=week")
	require.NotEmpty(t, report.Buckets)

	// Summing every bucket's net_movement must equal the change in the cash
	// scope's own balance over the same span — the reconciliation the plan
	// requires. The scope started empty, so the sum is the closing balance.
	summed := new(big.Int)
	for _, bucket := range report.Buckets {
		for _, total := range bucket.Totals {
			assertCashflowReconciles(t, total)
			summed.Add(summed, total.NetMovement.BigInt())
		}
	}

	balances := readAccountBalancesForSession(t, handler, sessionCookie, "?as_of=2026-06-30")
	cashBalance := new(big.Int)
	for _, account := range balances.Accounts {
		if account.AccountID != checking.ID && account.AccountID != savings.ID {
			continue
		}
		for _, balance := range account.DirectBalances {
			cashBalance.Add(cashBalance, balance.QuantityValue.BigInt())
		}
	}

	assert.Equal(t, cashBalance.String(), summed.String(),
		"summed net_movement must reconcile to the cash scope's balance change")
}

func TestCashflowNamesItsCashScopeAndExcludesNonCashAccounts(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, usdID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Scope Checking", "asset", "checking", usdID, 2)
	property := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Scope Property", "asset", "property", usdID, 2)

	report := readCashflowForSession(t, handler, sessionCookie, "?start_date=2026-06-01&end_date=2026-06-30&bucket=month")

	assert.Equal(t, []string{"cash", "checking", "savings", "brokerage_cash"}, report.ScopeKinds)

	scopeIDs := map[int64]bool{}
	for _, account := range report.ScopeAccounts {
		scopeIDs[account.AccountID] = true
	}
	assert.True(t, scopeIDs[checking.ID], "a checking account belongs to the default cash scope")
	assert.False(t, scopeIDs[property.ID], "a property account is not liquid cash")
}

func TestCashflowExcludesVoidedActivity(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, usdID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Voidflow Checking", "asset", "checking", usdID, 2)
	salary := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Voidflow Salary","category_type":"income"}`)

	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-02",
		posting(checking.ID, 50000, 2, usdID),
		posting(salary.ID, -50000, 2, usdID),
	), http.StatusCreated)
	voided := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-03",
		posting(checking.ID, 900000, 2, usdID),
		posting(salary.ID, -900000, 2, usdID),
	), http.StatusCreated)
	mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost,
		"/api/v1/transactions/"+strconvFormatInt(voided.ID)+"/void",
		`{"change_reason":"duplicate import"}`, http.StatusOK)

	report := readCashflowForSession(t, handler, sessionCookie, "?start_date=2026-06-01&end_date=2026-06-30&bucket=month")

	require.Len(t, report.Buckets, 1)
	require.Len(t, report.Buckets[0].Totals, 1)
	assert.Equal(t, "50000", report.Buckets[0].Totals[0].NetMovement.String())
}

func TestCashflowReturnsEmptyBucketsRatherThanOmittingPeriods(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, usdID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Gap Checking", "asset", "checking", usdID, 2)
	salary := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Gap Salary","category_type":"income"}`)

	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-02",
		posting(checking.ID, 50000, 2, usdID),
		posting(salary.ID, -50000, 2, usdID),
	), http.StatusCreated)

	report := readCashflowForSession(t, handler, sessionCookie, "?start_date=2026-06-01&end_date=2026-08-31&bucket=month")

	// A month with no activity is still a row: a gap in a cashflow table reads
	// as missing data rather than as a quiet month.
	require.Len(t, report.Buckets, 3)
	assert.Empty(t, report.Buckets[1].Totals)
	assert.Empty(t, report.Buckets[2].Totals)
}

func TestCashflowRejectsInvalidQuery(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, _, _ := setupAccountAPITest(t, handler)

	for _, suffix := range []string{
		"?end_date=2026-06-30",
		"?start_date=2026-06-30&end_date=2026-06-01",
		"?start_date=2026-06-01&end_date=2026-06-30&bucket=fortnight",
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/cashflow"+suffix, nil)
		req.AddCookie(sessionCookie)
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		assert.Equal(t, http.StatusBadRequest, res.Code, suffix)
	}
}

func TestCashflowRequiresAuthentication(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/cashflow?start_date=2026-06-01&end_date=2026-06-30", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	assert.Equal(t, http.StatusUnauthorized, res.Code)
}

func readCashflowForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, suffix string) cashflowResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/cashflow"+suffix, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	var response cashflowResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}
