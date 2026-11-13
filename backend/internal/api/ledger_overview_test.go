package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
