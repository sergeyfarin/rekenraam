package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #173: the named short sale and cover endpoints, their preview, reconciliation
// impact, realized-gains side and the stable side-conflict code.
func TestShortSaleAndCoverAPI(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "SHRT")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	post := func(path string, body any, status int) *http.Response {
		return doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, body, status).Result()
	}

	opening := tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 10000)
	post("/api/v1/investments/short-sale/reconciliation-impact", opening, http.StatusOK)
	res := post("/api/v1/investments/short-sale", opening, http.StatusCreated)
	var created investmentTradeResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
	require.NotNil(t, created.LotID)

	// A buy cannot net against the open short.
	res = post("/api/v1/investments/buy", tradeRequestBody(f, holding.ID, instrument.CommodityID, "1", 1000), http.StatusConflict)
	var conflict errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&conflict))
	assert.Equal(t, "INVESTMENT_POSITION_SIDE_CONFLICT", conflict.Error.Code)

	cover := tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 7000)
	cover.TransactionDate = "2026-03-01"
	res = post("/api/v1/investments/short-cover/preview", cover, http.StatusOK)
	var preview sellPreviewResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&preview))
	require.NotNil(t, preview.RealizedGain)
	require.NotNil(t, preview.RealizedGainScale)
	// 30.00 at the currency's allocation scale, as for a long sale (T-103).
	assert.Equal(t, moneyCoefficient(30000000), *preview.RealizedGain)
	assert.Equal(t, 6, *preview.RealizedGainScale)
	post("/api/v1/investments/short-cover/reconciliation-impact", cover, http.StatusOK)
	res = post("/api/v1/investments/short-cover", cover, http.StatusCreated)
	var covered investmentTradeResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&covered))
	require.NotNil(t, covered.DisposalDecision)
	assert.Equal(t, moneyCoefficient(-7000), covered.DisposalDecision.ProceedsValue)

	gainsRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodGet, "/api/v1/investments/gains", nil, http.StatusOK)
	var gains struct {
		Realized []realizedGainResponse `json:"realized"`
	}
	require.NoError(t, json.NewDecoder(gainsRes.Body).Decode(&gains))
	require.Len(t, gains.Realized, 1)
	assert.Equal(t, "short", gains.Realized[0].PositionSide)
	require.NotNil(t, gains.Realized[0].RealizedGainValue)
	require.NotNil(t, gains.Realized[0].RealizedGainScale)
	assert.Equal(t, []any{moneyCoefficient(30000000), 6}, []any{*gains.Realized[0].RealizedGainValue, *gains.Realized[0].RealizedGainScale})

	// Over-cover is a conflict; an opening behind the cover is out of order.
	res = post("/api/v1/investments/short-cover", cover, http.StatusConflict)
	require.NoError(t, json.NewDecoder(res.Body).Decode(&conflict))
	assert.Equal(t, "CONFLICT", conflict.Error.Code)
	post("/api/v1/investments/short-sale", opening, http.StatusConflict)
}
