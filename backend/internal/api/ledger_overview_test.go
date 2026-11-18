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

func TestLedgerOverviewHTTPComposesAccountMetadataAndBalances(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	cookie, csrf, currencyID := setupAccountAPITest(t, handler)
	account := createLedgerAccount(t, handler, cookie, csrf, "Overview checking", "asset", "checking", currencyID, 2)
	income := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Overview salary","category_type":"income"}`)
	createTransactionForSession(t, handler, cookie, csrf, balancedBody("2026-10-15", posting(account.ID, 12345, 2, currencyID), posting(income.ID, -12345, 2, currencyID)), http.StatusCreated)
	for _, path := range []string{"/api/v1/ledger/overview?as_of=2026-10-15", "/api/v1/ledger/accounts/" + strconvFormatInt(account.ID) + "?as_of=2026-10-15"} {
		req := httptest.NewRequest("GET", path, nil)
		req.AddCookie(cookie)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		require.Equal(t, 200, res.Code, res.Body.String())
		var body ledgerOverviewResponse
		require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
		require.Len(t, body.Accounts, 1)
		assert.Equal(t, account.ID, body.Accounts[0].Account.ID)
		assert.Equal(t, "Overview checking", body.Accounts[0].Account.Name)
		require.Len(t, body.Accounts[0].Balances, 1)
		assert.Equal(t, "12345", body.Accounts[0].Balances[0].QuantityValue.String())
		assert.Empty(t, body.Accounts[0].Positions)
		assert.Nil(t, body.NextCursor)
		assert.Equal(t, 60, body.ReconciliationOutOfDateAfterDays)
		require.NotNil(t, body.Accounts[0].Reconciliation)
		assert.Equal(t, "never_reconciled", body.Accounts[0].Reconciliation.State)
		assert.Nil(t, body.Accounts[0].Reconciliation.LastStatementDate)
	}
	for _, path := range []string{"/api/v1/ledger/overview?cursor=bad", "/api/v1/ledger/overview?as_of=2026-02-31", "/api/v1/ledger/accounts/nope"} {
		req := httptest.NewRequest("GET", path, nil)
		req.AddCookie(cookie)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		assert.Equal(t, 400, res.Code)
	}
	req := httptest.NewRequest("GET", "/api/v1/ledger/overview", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	assert.Equal(t, 401, res.Code)
}

// #482: the window is one book setting, read back by every judgement.
func TestReconciliationPolicyHTTPSavesTheWindowTheAccountListReads(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	cookie, csrf, currencyID := setupAccountAPITest(t, handler)
	createLedgerAccount(t, handler, cookie, csrf, "Window checking", "asset", "checking", currencyID, 2)

	read := func() reconciliationPolicyResponse {
		t.Helper()
		req := httptest.NewRequest("GET", "/api/v1/reconciliation/policy", nil)
		req.AddCookie(cookie)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		require.Equal(t, 200, res.Code, res.Body.String())
		var body reconciliationPolicyResponse
		require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
		return body
	}
	initial := read()
	assert.Equal(t, 60, initial.OutOfDateAfterDays)
	assert.Nil(t, initial.UpdatedAt)

	for _, body := range []string{`{"out_of_date_after_days":0}`, `{"out_of_date_after_days":3651}`, `{"out_of_date_after_days":"30"}`, `{"out_of_date_after_days":30,"extra":1}`} {
		mutateBackup(t, handler, cookie, csrf, "PUT", "/api/v1/reconciliation/policy", body, http.StatusBadRequest)
	}
	mutateBackup(t, handler, cookie, "", "PUT", "/api/v1/reconciliation/policy", `{"out_of_date_after_days":30}`, http.StatusForbidden)
	mutateBackup(t, handler, cookie, csrf, "PUT", "/api/v1/reconciliation/policy", `{"out_of_date_after_days":30}`, http.StatusOK)
	saved := read()
	assert.Equal(t, 30, saved.OutOfDateAfterDays)
	assert.NotNil(t, saved.UpdatedAt)

	req := httptest.NewRequest("GET", "/api/v1/ledger/overview", nil)
	req.AddCookie(cookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, 200, res.Code, res.Body.String())
	var overview ledgerOverviewResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&overview))
	assert.Equal(t, 30, overview.ReconciliationOutOfDateAfterDays)
	require.Len(t, overview.Accounts, 1)
	require.NotNil(t, overview.Accounts[0].Reconciliation)
	assert.Equal(t, "nothing_to_reconcile", overview.Accounts[0].Reconciliation.State, "an account without postings is not out of date")

	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest("PUT", "/api/v1/reconciliation/policy", strings.NewReader(`{"out_of_date_after_days":30}`)))
	assert.Equal(t, 401, unauthenticated.Code)
}
