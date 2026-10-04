package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// T-118: write-off reversal and replacement over HTTP, fenced from the sale
// commands, with the correction chain and context offering them.
func TestReplaceAndReverseWriteOffAPI(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "WOFF")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 100000), http.StatusCreated)
	writeOff := func(quantity int64) investmentWriteOffRequest {
		return investmentWriteOffRequest{TransactionDate: "2026-03-01", CommodityID: instrument.CommodityID,
			HoldingAccountID: holding.ID, QuantityValue: exact.New(quantity), Reason: "delisted", CostBasisMethod: "fifo"}
	}
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/write-off", writeOff(6), http.StatusCreated)
	var written investmentTradeResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&written))
	base := "/api/v1/investments/transactions/" + strconv.FormatInt(written.Transaction.ID, 10)

	chainRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, base+"/correction-chain", nil, http.StatusOK)
	var chain investmentCorrectionChainResponse
	require.NoError(t, json.NewDecoder(chainRes.Body).Decode(&chain))
	require.True(t, chain.CanCorrectWriteOff)
	require.False(t, chain.CanReverseSale)
	contextRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, base+"/trade-correction-context", nil, http.StatusOK)
	var source investmentTradeCorrectionContextResponse
	require.NoError(t, json.NewDecoder(contextRes.Body).Decode(&source))
	require.Equal(t, "write_off", source.OperationKind)
	require.Equal(t, "6", source.QuantityValue)
	require.True(t, source.CanReplaceSale)

	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base+"/reverse-sale",
		investmentSaleReversalRequest{Reason: "wrong command"}, http.StatusNotFound)
	replace := investmentWriteOffReplacementRequest{Reason: "four shares only", Replacement: writeOff(4)}
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, base+"/replace-write-off", replace, http.StatusForbidden)
	preview := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		base+"/replace-write-off/reconciliation-impact", replace, http.StatusOK)
	var impact reconciliationImpactResponse
	require.NoError(t, json.NewDecoder(preview.Body).Decode(&impact))
	require.Empty(t, impact.AffectedCheckpoints)
	require.NotNil(t, impact.GainImpact)
	require.Len(t, impact.GainImpact.Changes, 1, "the write-off's own loss is replaced")
	required := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base+"/replace-write-off", replace, http.StatusConflict)
	require.Contains(t, required.Body.String(), "INVESTMENT_GAIN_IMPACT_ACKNOWLEDGEMENT_REQUIRED")
	replace.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base+"/replace-write-off", replace, http.StatusCreated)
	var replaced investmentSaleReplacementResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&replaced))
	require.Equal(t, written.Transaction.ID, replaced.CorrectedTransactionID)
	conflict := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base+"/replace-write-off", replace, http.StatusConflict)
	require.Contains(t, conflict.Body.String(), "INVESTMENT_WRITE_OFF_ALREADY_CORRECTED")

	effective := "/api/v1/investments/transactions/" + strconv.FormatInt(replaced.Replacement.Transaction.ID, 10)
	reverse := investmentSaleReversalRequest{Reason: "the fund reopened"}
	preview = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, effective+"/reverse-write-off/reconciliation-impact", reverse, http.StatusOK)
	require.NoError(t, json.NewDecoder(preview.Body).Decode(&impact))
	reverse.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, effective+"/reverse-write-off", reverse, http.StatusCreated)
	chainRes = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, base+"/correction-chain", nil, http.StatusOK)
	require.NoError(t, json.NewDecoder(chainRes.Body).Decode(&chain))
	require.False(t, chain.CanCorrectWriteOff)
	require.Nil(t, chain.EffectiveTransactionID)
}
