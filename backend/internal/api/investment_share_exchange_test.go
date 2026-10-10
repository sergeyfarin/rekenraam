package api

import (
	"encoding/json"
	"fmt"
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
	require.Len(t, exchanged.Plan.BasisTotals, 1)
	assert.Equal(t, "known", exchanged.Plan.BasisTotals[0].BasisKnowledge)
	assert.Equal(t, *exchanged.Plan.Links[0].CarriedBasisValue, *exchanged.Plan.BasisTotals[0].CarriedBasisValue)

	// #178: the new lot names the exchange and keeps the original date; the
	// chain explains the exchange; the journal carries its title label.
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, fmt.Sprintf(
		"/api/v1/investments/lots?account_id=%d&commodity_id=%d", successorHolding.ID, successor.CommodityID), nil, http.StatusOK)
	var lots investmentLotsResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&lots))
	require.Len(t, lots.Lots, 1)
	origin := lots.Lots[0].Origin
	require.NotNil(t, origin)
	assert.Equal(t, "exchange", origin.TransferKind)
	assert.Equal(t, old.CommodityID, origin.SourceCommodityID)
	assert.Equal(t, holding.ID, *origin.SourceAccountID)
	assert.Equal(t, [2]int64{3, 2}, [2]int64{*origin.RatioNumerator, *origin.RatioDenominator})
	assert.Equal(t, "2026-01-01", *origin.OriginalAcquiredOn)
	assert.Equal(t, "2026-02-01", lots.Lots[0].OpenedOn)
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, fmt.Sprintf(
		"/api/v1/investments/transactions/%d/correction-chain", exchanged.Transaction.ID), nil, http.StatusOK)
	var chain investmentCorrectionChainResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&chain))
	require.NotNil(t, chain.EffectiveShareExchange)
	assert.Equal(t, successorHolding.ID, chain.EffectiveShareExchange.DestinationHoldingAccountID)
	assert.Equal(t, successor.CommodityID, chain.EffectiveShareExchange.DestinationCommodityID)
	assert.JSONEq(t, `{"notice":"merger"}`, string(chain.EffectiveShareExchange.SourceEvidence))
	assert.Equal(t, exchanged.Plan, chain.EffectiveShareExchange.Plan)
	assert.False(t, chain.CanReverseTransfer || chain.CanReplaceTransfer, "an exchange has its own correction")
	assert.True(t, chain.CanCorrectShareExchange)
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, fmt.Sprintf(
		"/api/v1/transactions/%d", exchanged.Transaction.ID), nil, http.StatusOK)
	assert.Contains(t, res.Body.String(), `"system_label":"share_exchange"`)

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

// #179: reversal and replacement endpoints, their previews and the named
// refusals: a sale of removed new units, a repeated correction, a missing
// exchange and unchanged terms.
func TestShareExchangeCorrectionAPI(t *testing.T) {
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
	exchange := func(date string) shareExchangeResponse {
		t.Helper()
		res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
			"/api/v1/investments/share-exchanges", shareExchangeRequest{EffectiveOn: date, HoldingAccountID: holding.ID,
				DestinationHoldingID: successorHolding.ID, CommodityID: old.CommodityID,
				DestinationCommodityID: successor.CommodityID, RatioNumerator: 1, RatioDenominator: 1}, http.StatusCreated)
		var exchanged shareExchangeResponse
		require.NoError(t, json.NewDecoder(res.Body).Decode(&exchanged))
		return exchanged
	}
	first := exchange("2026-02-01")
	path := func(id int64, action string) string {
		return fmt.Sprintf("/api/v1/investments/transactions/%d/%s", id, action)
	}

	// Replace 1:1 with 2:1: the preview and the commit carry 20 new units.
	replacement := shareExchangeReplacementRequest{Reason: "wrong ratio", EffectiveOn: "2026-02-01",
		DestinationHoldingID: successorHolding.ID, DestinationCommodityID: successor.CommodityID,
		RatioNumerator: 2, RatioDenominator: 1}
	res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		path(first.Transaction.ID, "replace-share-exchange/preview"), replacement, http.StatusOK)
	var preview shareExchangePreviewResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&preview))
	assert.Equal(t, "20", preview.Plan.DestinationQuantityValue.String())
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		path(first.Transaction.ID, "replace-share-exchange"), replacement, http.StatusForbidden)
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		path(first.Transaction.ID, "replace-share-exchange"), replacement, http.StatusCreated)
	var replaced shareExchangeReplacementResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&replaced))
	assert.Equal(t, first.Transaction.ID, replaced.CorrectedTransactionID)
	assert.Equal(t, "20", replaced.Plan.DestinationQuantityValue.String())
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		path(first.Transaction.ID, "replace-share-exchange"), replacement, http.StatusConflict)
	assert.Contains(t, res.Body.String(), "INVESTMENT_EXCHANGE_ALREADY_CORRECTED")
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		path(replaced.Replacement.ID, "replace-share-exchange"), replacement, http.StatusBadRequest)
	assert.Contains(t, res.Body.String(), "VALIDATION_FAILED", "unchanged terms")

	// A sale of the new units blocks reversing the replacement, by name.
	sale := tradeRequestBody(f, successorHolding.ID, successor.CommodityID, "5", 10000)
	sale.TransactionDate = "2026-03-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/sell", sale, http.StatusCreated)
	reversal := investmentSaleReversalRequest{Reason: "not a merger"}
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		path(replaced.Replacement.ID, "reverse-share-exchange/reconciliation-impact"), reversal, http.StatusConflict)
	assert.Contains(t, res.Body.String(), "INVESTMENT_TRANSFER_DEPENDENCY")
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		path(replaced.Replacement.ID, "reverse-share-exchange"), reversal, http.StatusConflict)
	assert.Contains(t, res.Body.String(), "INVESTMENT_TRANSFER_DEPENDENCY")
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		path(999999, "reverse-share-exchange"), reversal, http.StatusNotFound)
}

// #179: an exchange dated behind later sales of the old instrument. One the
// emptied holding leaves without units is named; one replay revises needs
// the preview's acknowledgement, then commits.
func TestShareExchangeBackdatedAPI(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	old := createInstrumentForSession(t, handler, f, "OLDCO")
	successor := createInstrumentForSession(t, handler, f, "NEWCO")
	holding := createHoldingAccountForSession(t, handler, f, old.ID)
	successorHolding := createHoldingAccountForSession(t, handler, f, successor.ID)
	trade := func(endpoint, date, quantity string) {
		t.Helper()
		body := tradeRequestBody(f, holding.ID, old.CommodityID, quantity, 10000)
		body.TransactionDate, body.CostBasisMethod = date, "fifo"
		doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, endpoint, body, http.StatusCreated)
	}
	trade("/api/v1/investments/buy", "2026-01-01", "10")
	trade("/api/v1/investments/sell", "2026-09-01", "3")
	request := shareExchangeRequest{EffectiveOn: "2026-06-01", HoldingAccountID: holding.ID,
		DestinationHoldingID: successorHolding.ID, CommodityID: old.CommodityID,
		DestinationCommodityID: successor.CommodityID, RatioNumerator: 1, RatioDenominator: 1}
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/share-exchanges", request, http.StatusConflict)
	assert.Contains(t, res.Body.String(), "INVESTMENT_TRANSFER_DEPENDENCY")

	// Units bought after the exchange date can carry the September sale.
	trade("/api/v1/investments/buy", "2026-08-01", "5")
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/share-exchanges", request, http.StatusConflict)
	assert.Contains(t, res.Body.String(), "INVESTMENT_GAIN_IMPACT_ACKNOWLEDGEMENT_REQUIRED")
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		"/api/v1/investments/share-exchanges/preview", request, http.StatusOK)
	var preview shareExchangePreviewResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&preview))
	require.NotNil(t, preview.Impact.GainImpact)
	assert.Equal(t, "10", preview.Plan.SourceQuantityValue.String(), "the holding open in June")
	request.GainImpactAcknowledgement = preview.Impact.GainImpact.Acknowledgement
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/share-exchanges", request, http.StatusCreated)
}
