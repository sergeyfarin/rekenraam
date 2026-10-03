package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvestmentSplitAPIPreviewCommitAndNamedRefusals(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "ISPLIT")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	buy := tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 10000)
	buy.TransactionDate = "2026-01-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", buy, http.StatusCreated)

	request := investmentSplitRequest{EffectiveOn: "2026-02-01", HoldingAccountID: holding.ID,
		CommodityID: instrument.CommodityID, RatioNumerator: 3, RatioDenominator: 2,
		SourceEvidence: json.RawMessage(`{"notice":"3-for-2"}`)}
	res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		"/api/v1/investments/splits/preview", request, http.StatusOK)
	var preview investmentSplitPreviewResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&preview))
	assert.Equal(t, "5", preview.Plan.QuantityDeltaValue.String())
	assert.Equal(t, 0, preview.Plan.QuantityDeltaScale)
	require.Len(t, preview.Plan.Effects, 1)
	assert.Equal(t, "15", preview.Plan.Effects[0].QuantityAfterValue.String())
	assert.Empty(t, preview.Impact.AffectedCheckpoints)

	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, "/api/v1/investments/splits", request, http.StatusForbidden)
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/splits", request, http.StatusCreated)
	var split investmentSplitResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&split))
	require.Len(t, split.Transaction.JournalEntries, 1)
	assert.Len(t, split.Transaction.JournalEntries[0].Postings, 2)

	// 15 × 1/7 has no exact decimal at the instrument's quantity scale.
	request.EffectiveOn, request.RatioNumerator, request.RatioDenominator = "2026-03-01", 1, 7
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/splits", request, http.StatusUnprocessableEntity)
	assert.Contains(t, res.Body.String(), "INVESTMENT_SPLIT_FRACTION_UNREPRESENTABLE")
	empty := createHoldingAccountForSession(t, handler, f, instrument.ID)
	request.HoldingAccountID, request.RatioNumerator, request.RatioDenominator = empty.ID, 2, 1
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/splits", request, http.StatusConflict)
	assert.Contains(t, res.Body.String(), "INVESTMENT_SPLIT_NO_HOLDINGS")
}
