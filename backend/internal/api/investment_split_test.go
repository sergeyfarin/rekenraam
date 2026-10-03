package api

import (
	"encoding/json"
	"net/http"
	"strconv"
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

func TestInvestmentSplitCorrectionAPIReplaceReverseAndLabels(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "ISPLITFIX")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	buy := tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 10000)
	buy.TransactionDate = "2026-01-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", buy, http.StatusCreated)
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/splits", investmentSplitRequest{EffectiveOn: "2026-02-01", HoldingAccountID: holding.ID,
			CommodityID: instrument.CommodityID, RatioNumerator: 2, RatioDenominator: 1}, http.StatusCreated)
	var split investmentSplitResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&split))
	splitPath := func(suffix string) string {
		return "/api/v1/investments/transactions/" + strconv.FormatInt(split.Transaction.ID, 10) + suffix
	}

	// A backdated purchase before the split posts a labelled adjustment.
	backdated := tradeRequestBody(f, holding.ID, instrument.CommodityID, "3", 3000)
	backdated.TransactionDate = "2026-01-15"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", backdated, http.StatusCreated)
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet,
		"/api/v1/accounts/"+strconv.FormatInt(holding.ID, 10)+"/register", nil, http.StatusOK)
	assert.Contains(t, res.Body.String(), `"system_label":"split_adjustment"`)

	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet,
		splitPath("/correction-chain"), nil, http.StatusOK)
	var chain investmentCorrectionChainResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&chain))
	assert.True(t, chain.CanCorrectSplit)
	require.NotNil(t, chain.EffectiveSplit)
	assert.Equal(t, int64(2), chain.EffectiveSplit.RatioNumerator)

	replacement := investmentSplitReplacementRequest{Reason: "it was 3-for-1", EffectiveOn: "2026-02-01",
		RatioNumerator: 3, RatioDenominator: 1}
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		splitPath("/replace-split/preview"), replacement, http.StatusOK)
	var preview investmentSplitPreviewResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&preview))
	assert.Equal(t, "26", preview.Plan.QuantityDeltaValue.String())
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, splitPath("/replace-split"), replacement, http.StatusForbidden)
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		splitPath("/replace-split"), replacement, http.StatusCreated)
	var replaced investmentSplitReplacementResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&replaced))
	assert.Equal(t, split.Transaction.ID, replaced.CorrectedTransactionID)

	// The original is now corrected; the replacement can be reversed.
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		splitPath("/reverse-split"), investmentSaleReversalRequest{Reason: "again"}, http.StatusConflict)
	assert.Contains(t, res.Body.String(), "INVESTMENT_SPLIT_ALREADY_CORRECTED")
	reversePath := "/api/v1/investments/transactions/" + strconv.FormatInt(replaced.Replacement.ID, 10) + "/reverse-split"
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		reversePath+"/reconciliation-impact", investmentSaleReversalRequest{Reason: "no split happened"}, http.StatusOK)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		reversePath, investmentSaleReversalRequest{Reason: "no split happened"}, http.StatusCreated)
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/transactions/999999/reverse-split", investmentSaleReversalRequest{Reason: "x"}, http.StatusNotFound)
	assert.Contains(t, res.Body.String(), "NOT_FOUND")
}
