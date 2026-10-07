package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

func TestCashInLieuAPIDisposesFractionAndNamesUnavailableSplit(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "ICIL")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	buy := tradeRequestBody(f, holding.ID, instrument.CommodityID, "3", 3000)
	buy.TransactionDate = "2026-01-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy", buy, http.StatusCreated)
	splitRequest := investmentSplitRequest{EffectiveOn: "2026-02-01", HoldingAccountID: holding.ID,
		CommodityID: instrument.CommodityID, RatioNumerator: 3, RatioDenominator: 2}
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/splits", splitRequest, http.StatusCreated)
	var split investmentSplitResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&split))

	path := "/api/v1/investments/cash-in-lieu"
	request := cashInLieuRequest{SplitTransactionID: split.Transaction.ID, DisposalOn: "2026-02-01", PaymentOn: "2026-02-05",
		QuantityValue: exact.New(5), QuantityScale: 1, CashAccountID: f.cashAccount.ID, CurrencyID: f.commodityID,
		ProceedsValue: 800, ProceedsScale: 2}
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, path+"/preview", request, http.StatusOK)
	var preview cashInLieuPreviewResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&preview))
	require.Len(t, preview.Allocations, 1)
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, path, request, http.StatusForbidden)
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, request, http.StatusCreated)
	var posted investmentTradeResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&posted))
	require.NotNil(t, posted.DisposalDecision)
	require.Len(t, posted.Transaction.JournalEntries, 2)

	request.SplitTransactionID = posted.Transaction.ID
	refused := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, request, http.StatusConflict)
	assert.Contains(t, refused.Body.String(), "INVESTMENT_CASH_IN_LIEU_SPLIT_UNAVAILABLE", "a non-split transaction is not a split")
}

func TestCashInLieuAPICorrectReverseAndReleaseSplitFence(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "CILC")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	buy := tradeRequestBody(f, holding.ID, instrument.CommodityID, "3", 3000)
	buy.TransactionDate = "2026-01-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy", buy, http.StatusCreated)
	splitRequest := investmentSplitRequest{EffectiveOn: "2026-02-01", HoldingAccountID: holding.ID, CommodityID: instrument.CommodityID, RatioNumerator: 3, RatioDenominator: 2}
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/splits", splitRequest, http.StatusCreated)
	var split investmentSplitResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&split))
	input := cashInLieuRequest{SplitTransactionID: split.Transaction.ID, DisposalOn: "2026-02-01", PaymentOn: "2026-02-05", QuantityValue: exact.New(5), QuantityScale: 1, CashAccountID: f.cashAccount.ID, CurrencyID: f.commodityID, ProceedsValue: 800, ProceedsScale: 2}
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, "/api/v1/investments/cash-in-lieu/preview", input, http.StatusOK)
	var initialPreview cashInLieuPreviewResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&initialPreview))
	require.Zero(t, exact.ScaledIntFromCoefficient(initialPreview.DisposedBasisValue, initialPreview.DisposedBasisScale).Cmp(exact.ScaledIntFromInt64(3333333, 6)))
	require.Zero(t, exact.ScaledIntFromCoefficient(initialPreview.RealizedGainValue, initialPreview.RealizedGainScale).Cmp(exact.ScaledIntFromInt64(4666667, 6)))
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/cash-in-lieu", input, http.StatusCreated)
	var posted investmentTradeResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&posted))
	contextPath := fmt.Sprintf("/api/v1/investments/transactions/%d/trade-correction-context", posted.Transaction.ID)
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, contextPath, nil, http.StatusOK)
	var source investmentTradeCorrectionContextResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&source))
	require.Equal(t, split.Transaction.ID, source.SplitTransactionID)
	require.False(t, source.CanReplaceSale, "generic sale correction must not be offered for cash in lieu")
	require.Len(t, source.AvailableLots, 1)
	correctionPath := fmt.Sprintf("/api/v1/investments/transactions/%d/replace-cash-in-lieu", posted.Transaction.ID)
	input.QuantityValue, input.QuantityScale, input.ProceedsValue, input.DisposalOn, input.PaymentOn = exact.New(25), 2, 850, "2026-02-02", "2026-02-08"
	input.CostBasisMethod = "specific_lot"
	input.LotAllocations = []investmentLotAllocationRequest{{LotID: source.AvailableLots[0].LotID, QuantityValue: input.QuantityValue, QuantityScale: input.QuantityScale}}
	replacement := cashInLieuReplacementRequest{Reason: "fraction and payment corrected", Replacement: input}
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, correctionPath+"/preview", replacement, http.StatusOK)
	var preview cashInLieuPreviewResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&preview))
	require.NotNil(t, preview.Impact.GainImpact)
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, correctionPath, replacement, http.StatusForbidden)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, correctionPath, replacement, http.StatusConflict)
	replacement.GainImpactAcknowledgement = preview.Impact.GainImpact.Acknowledgement
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, correctionPath, replacement, http.StatusCreated)
	var corrected investmentSaleReplacementResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&corrected))
	require.Equal(t, preview.Allocations, corrected.Replacement.Allocations)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, correctionPath, replacement, http.StatusConflict)
	// Effective successor retains the split fence; reversing it releases it.
	splitReversePath := fmt.Sprintf("/api/v1/investments/transactions/%d/reverse-split/reconciliation-impact", split.Transaction.ID)
	reason := investmentSaleReversalRequest{Reason: "withdrawn"}
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, splitReversePath, reason, http.StatusConflict)
	chainPath := fmt.Sprintf("/api/v1/investments/transactions/%d/correction-chain", corrected.Replacement.Transaction.ID)
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, chainPath, nil, http.StatusOK)
	var chain investmentCorrectionChainResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&chain))
	require.True(t, chain.CanCorrectCashInLieu)
	reversePath := fmt.Sprintf("/api/v1/investments/transactions/%d/reverse-cash-in-lieu", corrected.Replacement.Transaction.ID)
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, reversePath+"/reconciliation-impact", reason, http.StatusOK)
	var impact reconciliationImpactResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&impact))
	if impact.GainImpact != nil {
		reason.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	}
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, reversePath, reason, http.StatusForbidden)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, reversePath, reason, http.StatusCreated)
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, splitReversePath, investmentSaleReversalRequest{Reason: "withdrawn"}, http.StatusOK)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, fmt.Sprintf("/api/v1/investments/transactions/%d/reverse-cash-in-lieu", split.Transaction.ID), reason, http.StatusNotFound)
}
