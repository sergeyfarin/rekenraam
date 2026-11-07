package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvestmentEntryAccountsAPI(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	path := "/api/v1/investments/entry-accounts?as_of=2026-01-01"
	unauth := httptest.NewRecorder()
	handler.ServeHTTP(unauth, httptest.NewRequest(http.MethodGet, path, nil))
	assert.Equal(t, http.StatusUnauthorized, unauth.Code)
	res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, path, nil, http.StatusOK)
	var response struct {
		Accounts []investmentEntryAccountResponse `json:"accounts"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	found := false
	for _, row := range response.Accounts {
		assert.False(t, row.IsSystem)
		if row.ID == f.cashAccount.ID {
			found = true
			assert.True(t, row.InvestmentSettlement)
			assert.False(t, row.InvestmentHolding)
			assert.Equal(t, f.commodityID, *row.DefaultCommodityID)
		}
	}
	require.True(t, found)
	for _, path := range []string{"/api/v1/investments/entry-accounts", "/api/v1/investments/entry-accounts?as_of=2026-02-30"} {
		doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, path, nil, http.StatusBadRequest)
	}
}
