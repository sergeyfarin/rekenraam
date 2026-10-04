package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// T-119: transfer reversal over HTTP. The correction chain offers it; a
// destination sale of the moved units refuses it by name; reversing the
// sale first lets it through.
func TestReverseInternalTransferAPI(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "XREV")
	source := createHoldingAccountForSession(t, handler, f, instrument.ID)
	destination := createHoldingAccountForSession(t, handler, f, instrument.ID)
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", tradeRequestBody(f, source.ID, instrument.CommodityID, "2", 20000), http.StatusCreated)
	var bought investmentTradeResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&bought))
	require.NotNil(t, bought.LotID)
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/transfers/internal",
		internalTransferRequest{EffectiveOn: "2026-02-01", SourceAccountID: source.ID, DestinationAccountID: destination.ID,
			CommodityID: instrument.CommodityID, CostCommodityID: f.commodityID,
			LotAllocations: []investmentLotAllocationRequest{{LotID: *bought.LotID, QuantityValue: exact.New(2)}},
		}, http.StatusCreated)
	var transfer internalTransferResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&transfer))
	base := "/api/v1/investments/transactions/" + strconv.FormatInt(transfer.Transaction.ID, 10)

	chainRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, base+"/correction-chain", nil, http.StatusOK)
	var chain investmentCorrectionChainResponse
	require.NoError(t, json.NewDecoder(chainRes.Body).Decode(&chain))
	require.True(t, chain.CanReverseTransfer)
	require.False(t, chain.CanReverseSale)

	sale := tradeRequestBody(f, destination.ID, instrument.CommodityID, "1", 15000)
	sale.TransactionDate = "2026-03-01"
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/sell", sale, http.StatusCreated)
	var sold investmentTradeResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&sold))
	reverse := investmentSaleReversalRequest{Reason: "moved by mistake"}
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, base+"/reverse-transfer", reverse, http.StatusForbidden)
	refused := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		base+"/reverse-transfer/reconciliation-impact", reverse, http.StatusConflict)
	require.Contains(t, refused.Body.String(), "INVESTMENT_TRANSFER_DEPENDENCY")

	saleBase := "/api/v1/investments/transactions/" + strconv.FormatInt(sold.Transaction.ID, 10)
	saleReversal := investmentSaleReversalRequest{Reason: "sold from the wrong account"}
	preview := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		saleBase+"/reverse-sale/reconciliation-impact", saleReversal, http.StatusOK)
	var impact reconciliationImpactResponse
	require.NoError(t, json.NewDecoder(preview.Body).Decode(&impact))
	require.NotNil(t, impact.GainImpact)
	saleReversal.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, saleBase+"/reverse-sale", saleReversal, http.StatusCreated)
	preview = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		base+"/reverse-transfer/reconciliation-impact", reverse, http.StatusOK)
	impact = reconciliationImpactResponse{}
	require.NoError(t, json.NewDecoder(preview.Body).Decode(&impact))
	require.Empty(t, impact.AffectedCheckpoints)
	if impact.GainImpact != nil {
		reverse.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	}
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base+"/reverse-transfer", reverse, http.StatusCreated)
	var reversed investmentSaleReversalResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&reversed))
	require.Equal(t, transfer.Transaction.ID, reversed.CorrectedTransactionID)
	require.Equal(t, "2026-02-01", reversed.Transaction.TransactionDate)
	conflict := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base+"/reverse-transfer", reverse, http.StatusConflict)
	require.Contains(t, conflict.Body.String(), "INVESTMENT_TRANSFER_ALREADY_CORRECTED")
	chainRes = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, base+"/correction-chain", nil, http.StatusOK)
	require.NoError(t, json.NewDecoder(chainRes.Body).Decode(&chain))
	require.False(t, chain.CanReverseTransfer)
	require.Nil(t, chain.EffectiveTransactionID)
}

// T-119: internal transfer replacement over HTTP. The preview carries the plan
// the commit records, with no durable lot IDs.
func TestReplaceInternalTransferAPIPreviewMatchesCommit(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "XREP")
	source := createHoldingAccountForSession(t, handler, f, instrument.ID)
	wrong := createHoldingAccountForSession(t, handler, f, instrument.ID)
	right := createHoldingAccountForSession(t, handler, f, instrument.ID)
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", tradeRequestBody(f, source.ID, instrument.CommodityID, "3", 30000), http.StatusCreated)
	var bought investmentTradeResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&bought))
	move := func(destination int64, quantity int64) internalTransferRequest {
		return internalTransferRequest{EffectiveOn: "2026-02-01", SourceAccountID: source.ID, DestinationAccountID: destination,
			CommodityID: instrument.CommodityID, CostCommodityID: f.commodityID,
			LotAllocations: []investmentLotAllocationRequest{{LotID: *bought.LotID, QuantityValue: exact.New(quantity)}}}
	}
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/transfers/internal", move(wrong.ID, 1), http.StatusCreated)
	var transfer internalTransferResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&transfer))
	base := "/api/v1/investments/transactions/" + strconv.FormatInt(transfer.Transaction.ID, 10)

	chainRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, base+"/correction-chain", nil, http.StatusOK)
	var chain investmentCorrectionChainResponse
	require.NoError(t, json.NewDecoder(chainRes.Body).Decode(&chain))
	require.True(t, chain.CanReplaceTransfer)
	require.NotNil(t, chain.EffectiveTransfer)
	require.Equal(t, "2026-02-01", chain.EffectiveTransfer.EffectiveOn)
	require.Equal(t, wrong.ID, chain.EffectiveTransfer.DestinationAccountID)
	require.Equal(t, "selected_lots", chain.EffectiveTransfer.BasisAllocation)
	require.Len(t, chain.EffectiveTransfer.LotAllocations, 1)
	require.Equal(t, *bought.LotID, chain.EffectiveTransfer.LotAllocations[0].LotID)
	require.Nil(t, chain.EffectiveTransfer.QuantityValue)
	replace := investmentTransferReplacementRequest{Reason: "two units, other account", Replacement: move(right.ID, 2)}
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, base+"/replace-transfer", replace, http.StatusForbidden)
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, base+"/replace-transfer/preview", replace, http.StatusOK)
	var preview internalTransferPreviewResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&preview))
	require.Len(t, preview.Plan.Links, 1)
	require.Nil(t, preview.Plan.Links[0].DestinationLotID)
	require.Empty(t, preview.Impact.AffectedCheckpoints)
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base+"/replace-transfer", replace, http.StatusCreated)
	var replaced investmentTransferReplacementResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&replaced))
	require.Equal(t, transfer.Transaction.ID, replaced.CorrectedTransactionID)
	require.Len(t, replaced.Replacement.Plan.Links, 1)
	require.Equal(t, preview.Plan.Links[0].CarriedBasisValue, replaced.Replacement.Plan.Links[0].CarriedBasisValue)
	require.Equal(t, preview.Plan.Links[0].QuantityValue, replaced.Replacement.Plan.Links[0].QuantityValue)
	conflict := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base+"/replace-transfer", replace, http.StatusConflict)
	require.Contains(t, conflict.Body.String(), "INVESTMENT_TRANSFER_ALREADY_CORRECTED")

	chainRes = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, base+"/correction-chain", nil, http.StatusOK)
	chain = investmentCorrectionChainResponse{}
	require.NoError(t, json.NewDecoder(chainRes.Body).Decode(&chain))
	require.NotNil(t, chain.EffectiveTransactionID)
	require.Equal(t, replaced.Replacement.Transaction.ID, *chain.EffectiveTransactionID)
	require.True(t, chain.CanReverseTransfer)
	require.True(t, chain.CanReplaceTransfer)
	require.Equal(t, right.ID, chain.EffectiveTransfer.DestinationAccountID)
}
