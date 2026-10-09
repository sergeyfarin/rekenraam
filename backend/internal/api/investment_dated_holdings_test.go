package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #166: one request returns the holdings an entry dated as_of would find,
// including one a later sale closed.
func TestDatedHoldingsAPI(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "DATED")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	buy := tradeRequestBody(f, holding.ID, instrument.CommodityID, "3", 3000)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy", buy, http.StatusCreated)
	sale := tradeRequestBody(f, holding.ID, instrument.CommodityID, "3", 4500)
	sale.TransactionDate = "2026-03-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/sell", sale, http.StatusCreated)

	var response datedHoldingsResponse
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodGet, "/api/v1/investments/dated-holdings?as_of=2026-02-15", nil, http.StatusOK)
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	require.Len(t, response.Holdings, 1)
	assert.Equal(t, "3", response.Holdings[0].QuantityValue.String())
	assert.Equal(t, "long", response.Holdings[0].PositionSide)
	require.Len(t, response.Holdings[0].Lots, 1)

	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodGet, "/api/v1/investments/dated-holdings?as_of=2026-03-01", nil, http.StatusOK)
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	assert.Empty(t, response.Holdings)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodGet, "/api/v1/investments/dated-holdings?as_of=nope", nil, http.StatusBadRequest)
}
