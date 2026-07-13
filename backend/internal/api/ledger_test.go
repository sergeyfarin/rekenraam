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

func TestLedgerReadModelBalancesTotalsAndRunningRegister(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, usdID := setupAccountAPITest(t, handler)
	mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/setup/system-accounts", http.StatusCreated)
	systemAccounts := listAccountsForSession(t, handler, sessionCookie, "?include_system=true")
	transferClearing := accountBySystemRole(t, systemAccounts.Accounts, "transfer_clearing")

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
	groceries := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Groceries","category_type":"expense"}`)
	salary := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Salary","category_type":"income"}`)

	salaryTx := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, 100000, 2, usdID),
		posting(salary.ID, -100000, 2, usdID),
	), http.StatusCreated)
	groceryTx := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -10000, 2, usdID),
		posting(groceries.ID, 10000, 2, usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(card.ID, -5000, 2, usdID),
		posting(groceries.ID, 5000, 2, usdID),
	), http.StatusCreated)
	cardPaymentTx := createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -5000, 2, usdID),
		posting(card.ID, 5000, 2, usdID),
	), http.StatusCreated)
	delayedTransferTx := createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"transaction_kind":"transfer",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"entry_kind":"transfer_leg",
			"postings":[
				`+posting(checking.ID, -10000, 2, usdID)+`,
				`+posting(transferClearing.ID, 10000, 2, usdID)+`
			]
		},{
			"entry_date":"2026-06-08",
			"entry_kind":"transfer_leg",
			"postings":[
				`+posting(transferClearing.ID, -10000, 2, usdID)+`,
				`+posting(checking.ID, 10000, 2, usdID)+`
			]
		}]
	}`, http.StatusCreated)

	balances := readAccountBalancesForSession(t, handler, sessionCookie, "?as_of=2026-06-07")
	checkingBalance := accountBalanceByID(t, balances.Accounts, checking.ID)
	assertBalance(t, checkingBalance.DirectBalances, usdID, 75000, 2, 75000)
	parentBalance := accountBalanceByID(t, balances.Accounts, cashParent.ID)
	assertBalance(t, parentBalance.SubtreeBalances, usdID, 75000, 2, 75000)
	for _, account := range balances.Accounts {
		assert.False(t, account.IsSystem)
	}

	systemBalances := readAccountBalancesForSession(t, handler, sessionCookie, "?as_of=2026-06-07&include_system=true")
	clearingBalance := accountBalanceByID(t, systemBalances.Accounts, transferClearing.ID)
	assertBalance(t, clearingBalance.DirectBalances, usdID, 10000, 2, 10000)

	categoryTotals := readCategoryTotalsForSession(t, handler, sessionCookie, "?after_date=2026-06-01&before_date=2026-06-30")
	groceryTotal := categoryTotalByID(t, categoryTotals.Categories, groceries.ID)
	assertBalance(t, groceryTotal.DirectTotals, usdID, 15000, 2, 15000)
	salaryTotal := categoryTotalByID(t, categoryTotals.Categories, salary.ID)
	assertBalance(t, salaryTotal.DirectTotals, usdID, -100000, 2, 100000)

	netWorth := readNetWorthForSession(t, handler, sessionCookie, "?as_of=2026-06-07")
	assertBalance(t, netWorth.Totals, usdID, 85000, 2, 85000)
	assert.Contains(t, netWorth.ExcludedSystemRoles, "commodity_trading")

	register := accountRegisterForSession(t, handler, sessionCookie, checking.ID, "?after_date=2026-06-07&before_date=2026-06-07")
	assertRegisterRunningBalance(t, register, salaryTx.ID, usdID, 100000)
	assertRegisterRunningBalance(t, register, groceryTx.ID, usdID, 90000)
	assertRegisterRunningBalance(t, register, cardPaymentTx.ID, usdID, 85000)
	assertRegisterRunningBalance(t, register, delayedTransferTx.ID, usdID, 75000)
}

func TestNetWorthSeriesUsesCalendarBucketEndsAndPostedLedgerOnly(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, usdID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Report Checking", "asset", "checking", usdID, 2)
	salary := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Report Salary","category_type":"income"}`)
	groceries := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Report Groceries","category_type":"expense"}`)

	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-03",
		posting(checking.ID, 100000, 2, usdID),
		posting(salary.ID, -100000, 2, usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-10",
		posting(checking.ID, -30000, 2, usdID),
		posting(groceries.ID, 30000, 2, usdID),
	), http.StatusCreated)

	series := readNetWorthSeriesForSession(t, handler, sessionCookie, "?start_date=2026-06-01&end_date=2026-06-14&bucket=week")
	require.Equal(t, "2026-06-01", series.StartDate)
	require.Equal(t, "2026-06-14", series.EndDate)
	require.Equal(t, "week", series.Bucket)
	assert.Equal(t, "2026-06-01", series.Query.StartDate)
	assert.Equal(t, "2026-06-14", series.Query.EndDate)
	assert.Equal(t, "week", series.Query.Bucket)
	require.Len(t, series.Buckets, 2)
	assert.Equal(t, "2026-06-01", series.Buckets[0].StartDate)
	assert.Equal(t, "2026-06-07", series.Buckets[0].EndDate)
	assertBalance(t, series.Buckets[0].Totals, usdID, 100000, 2, 100000)
	assert.Equal(t, "2026-06-08", series.Buckets[1].StartDate)
	assert.Equal(t, "2026-06-14", series.Buckets[1].EndDate)
	assertBalance(t, series.Buckets[1].Totals, usdID, 70000, 2, 70000)
	assert.Contains(t, series.ExcludedSystemRoles, "commodity_trading")
}

func TestNetWorthSeriesRejectsIncompleteOrInvalidQuery(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, _, _ := setupAccountAPITest(t, handler)

	for _, suffix := range []string{
		"?start_date=2026-06-01&bucket=month",
		"?start_date=2026-06-02&end_date=2026-06-01&bucket=month",
		"?start_date=2026-06-01&end_date=2026-06-30&bucket=fortnight",
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/net-worth"+suffix, nil)
		req.AddCookie(sessionCookie)
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		assert.Equal(t, http.StatusBadRequest, res.Code, suffix)
	}
}

func readAccountBalancesForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, suffix string) accountBalancesResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ledger/account-balances"+suffix, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	var response accountBalancesResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}

func readCategoryTotalsForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, suffix string) categoryTotalsResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ledger/category-totals"+suffix, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	var response categoryTotalsResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}

func readNetWorthForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, suffix string) netWorthResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ledger/net-worth"+suffix, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	var response netWorthResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}

func readNetWorthSeriesForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, suffix string) netWorthSeriesResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/net-worth"+suffix, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	var response netWorthSeriesResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}

func accountBalanceByID(t *testing.T, accounts []accountBalanceResponse, accountID int64) accountBalanceResponse {
	t.Helper()

	for _, account := range accounts {
		if account.AccountID == accountID {
			return account
		}
	}
	require.Failf(t, "account balance not found", "account_id=%d", accountID)
	return accountBalanceResponse{}
}

func categoryTotalByID(t *testing.T, categories []categoryTotalResponse, categoryID int64) categoryTotalResponse {
	t.Helper()

	for _, category := range categories {
		if category.CategoryID == categoryID {
			return category
		}
	}
	require.Failf(t, "category total not found", "category_id=%d", categoryID)
	return categoryTotalResponse{}
}

func assertBalance(t *testing.T, balances []balanceQuantityResponse, commodityID int64, quantityValue int64, quantityScale int, normalQuantityValue int64) {
	t.Helper()

	seen := make([]string, 0, len(balances))
	for _, balance := range balances {
		seen = append(seen, strconvFormatInt(balance.CommodityID)+":"+balance.QuantityValue.String()+":"+strconvFormatInt(int64(balance.QuantityScale))+":"+balance.NormalQuantityValue.String())
		if balance.CommodityID == commodityID {
			assert.Equal(t, strconvFormatInt(quantityValue), balance.QuantityValue.String())
			assert.Equal(t, quantityScale, balance.QuantityScale)
			assert.Equal(t, strconvFormatInt(normalQuantityValue), balance.NormalQuantityValue.String())
			return
		}
	}
	require.Failf(t, "balance not found", "commodity_id=%d seen=%s", commodityID, strings.Join(seen, ","))
}

func assertRegisterRunningBalance(t *testing.T, register accountRegisterResponse, transactionID int64, commodityID int64, quantityValue int64) {
	t.Helper()

	for _, entry := range register.Entries {
		if entry.TransactionID == transactionID && entry.Posting.CommodityID == commodityID {
			assert.Equal(t, strconvFormatInt(quantityValue), entry.RunningBalance.QuantityValue.String())
			assert.Equal(t, commodityID, entry.RunningBalance.CommodityID)
			return
		}
	}
	require.Failf(t, "register entry not found", "transaction_id=%d commodity_id=%d", transactionID, commodityID)
}
