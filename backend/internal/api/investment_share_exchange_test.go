package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #177: the share exchange endpoints, their preview and the named refusals.
func TestShareExchangeAPIPreviewCommitAndNamedRefusals(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	old := createInstrumentForSession(t, handler, f, "OLDCO")
	successor := createInstrumentForSession(t, handler, f, "NEWCO")
	holding := createHoldingAccountForSession(t, handler, f, old.ID)
	successorHolding := createHoldingAccountForSession(t, handler, f, successor.ID)
	buy := tradeRequestBody(f, holding.ID, old.CommodityID, "10", 10000)
	buy.TransactionDate = "2026-01-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", buy, http.StatusCreated)

	request := shareExchangeRequest{EffectiveOn: "2026-02-01", HoldingAccountID: holding.ID,
		CommodityID: old.CommodityID, DestinationCommodityID: successor.CommodityID,
		RatioNumerator: 6, RatioDenominator: 4, SourceEvidence: json.RawMessage(`{"notice":"merger"}`)}
	// A holding whose default instrument is the old one cannot hold the new
	// one: the new units go to the new instrument's own holding.
	res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		"/api/v1/investments/share-exchanges/preview", request, http.StatusBadRequest)
	assert.Contains(t, res.Body.String(), "default commodity")
	request.DestinationHoldingID = successorHolding.ID
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		"/api/v1/investments/share-exchanges/preview", request, http.StatusOK)
	var preview shareExchangePreviewResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&preview))
	assert.Equal(t, [2]int64{3, 2}, [2]int64{preview.Plan.RatioNumerator, preview.Plan.RatioDenominator}, "stored in lowest terms")
	assert.Equal(t, "10", preview.Plan.SourceQuantityValue.String())
	assert.Equal(t, "15", preview.Plan.DestinationQuantityValue.String())
	require.Len(t, preview.Plan.Links, 1)
	assert.Nil(t, preview.Plan.Links[0].DestinationLotID)
	require.NotNil(t, preview.Plan.Links[0].CarriedBasisValue)
	assert.Equal(t, "known", preview.Plan.Links[0].BasisKnowledge)
	assert.Equal(t, "2026-01-01", *preview.Plan.Links[0].OriginalAcquiredOn)
	assert.Empty(t, preview.Impact.AffectedCheckpoints)

	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, "/api/v1/investments/share-exchanges", request, http.StatusForbidden)
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/share-exchanges", request, http.StatusCreated)
	var exchanged shareExchangeResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&exchanged))
	require.Len(t, exchanged.Transaction.JournalEntries, 1)
	assert.Len(t, exchanged.Transaction.JournalEntries[0].Postings, 4)
	require.Len(t, exchanged.Plan.Links, 1)
	require.NotNil(t, exchanged.Plan.Links[0].DestinationLotID)
	assert.Equal(t, *preview.Plan.Links[0].CarriedBasisValue, *exchanged.Plan.Links[0].CarriedBasisValue)

	// Nothing of the old instrument is left to exchange again.
	request.EffectiveOn = "2026-03-01"
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/share-exchanges", request, http.StatusConflict)
	assert.Contains(t, res.Body.String(), "INVESTMENT_EXCHANGE_NO_HOLDINGS")
	// 15 × 1/7 has no exact decimal at the new instrument's quantity scale.
	request.CommodityID, request.DestinationCommodityID = successor.CommodityID, old.CommodityID
	request.HoldingAccountID, request.DestinationHoldingID = successorHolding.ID, holding.ID
	request.RatioNumerator, request.RatioDenominator = 1, 7
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/share-exchanges", request, http.StatusUnprocessableEntity)
	assert.Contains(t, res.Body.String(), "INVESTMENT_EXCHANGE_FRACTION_UNREPRESENTABLE")
	request.DestinationCommodityID = successor.CommodityID
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/share-exchanges", request, http.StatusBadRequest)
	assert.Contains(t, res.Body.String(), "VALIDATION_FAILED")
}
