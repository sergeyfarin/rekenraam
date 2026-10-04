package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// T-115: wire contract for dividend and reinvested-dividend corrections.

func expectInvestmentConflict(t *testing.T, handler http.Handler, f investmentAPITestFixture, path string, body any, code string) {
	t.Helper()
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, body, http.StatusConflict)
	var envelope errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&envelope))
	assert.Equal(t, code, envelope.Error.Code)
}

func TestReplaceAndReverseDividendAPI(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	dividend := func(amount int64) dividendRequest {
		return dividendRequest{TransactionDate: "2026-04-01", CashAccountID: f.cashAccount.ID, CashCommodityID: f.commodityID,
			IncomeAccountID: &f.incomeAccount.ID, AmountValue: moneyCoefficient(amount), AmountScale: 2}
	}
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/dividend", dividend(5000), http.StatusCreated)
	var original transactionResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&original))
	base := "/api/v1/investments/transactions/" + strconv.FormatInt(original.ID, 10)
	replace := investmentDividendReplacementRequest{Reason: "statement shows 55.00", Replacement: dividend(5500)}

	previewRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, base+"/replace-dividend/reconciliation-impact", replace, http.StatusOK)
	var preview map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(previewRes.Body).Decode(&preview))
	assert.JSONEq(t, `[]`, string(preview["affected_checkpoints"]))
	_, hasGainImpact := preview["gain_impact"]
	assert.False(t, hasGainImpact, "a cash dividend replays no disposal")

	replacedRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base+"/replace-dividend", replace, http.StatusCreated)
	var replaced investmentDividendReplacementResponse
	require.NoError(t, json.NewDecoder(replacedRes.Body).Decode(&replaced))
	assert.Equal(t, original.ID, replaced.CorrectedTransactionID)
	require.NotNil(t, replaced.InverseTransaction.CorrectionOfTransactionID)
	assert.Equal(t, original.ID, *replaced.InverseTransaction.CorrectionOfTransactionID)
	assert.Equal(t, original.ID, *replaced.ReplacementTransaction.CorrectionOfTransactionID)

	expectInvestmentConflict(t, handler, f, base+"/replace-dividend", replace, "INVESTMENT_DIVIDEND_ALREADY_CORRECTED")
	expectInvestmentConflict(t, handler, f, base+"/reverse-dividend", investmentSaleReversalRequest{Reason: "again"}, "INVESTMENT_DIVIDEND_ALREADY_CORRECTED")
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base+"/reverse-dividend", investmentSaleReversalRequest{}, http.StatusBadRequest)

	replacementBase := "/api/v1/investments/transactions/" + strconv.FormatInt(replaced.ReplacementTransaction.ID, 10)
	reversedRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, replacementBase+"/reverse-dividend",
		investmentSaleReversalRequest{Reason: "paid in error"}, http.StatusCreated)
	var reversed investmentSaleReversalResponse
	require.NoError(t, json.NewDecoder(reversedRes.Body).Decode(&reversed))
	assert.Equal(t, replaced.ReplacementTransaction.ID, reversed.CorrectedTransactionID)
	// The reversal command is fenced to dividends.
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		"/api/v1/investments/transactions/"+strconv.FormatInt(reversed.Transaction.ID, 10)+"/reverse-reinvested-dividend/reconciliation-impact",
		investmentSaleReversalRequest{Reason: "wrong family"}, http.StatusNotFound)
}

func TestReplaceAndReverseReinvestedDividendAPI(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "REIN")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	reinvestment := func(quantity int64, amount int64) reinvestedDividendRequest {
		return reinvestedDividendRequest{TransactionDate: "2026-01-02", CommodityID: instrument.CommodityID, HoldingAccountID: holding.ID,
			IncomeAccountID: &f.incomeAccount.ID, QuantityValue: exact.New(quantity), QuantityScale: 0,
			AmountValue: moneyCoefficient(amount), AmountScale: 2, CashCommodityID: f.commodityID}
	}
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/reinvested-dividend", reinvestment(10, 2000), http.StatusCreated)
	var original investmentTradeResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&original))
	buy := tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 20000)
	buy.TransactionDate = "2026-02-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy", buy, http.StatusCreated)
	sale := tradeRequestBody(f, holding.ID, instrument.CommodityID, "5", 15000)
	sale.TransactionDate = "2026-03-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/sell", sale, http.StatusCreated)

	base := "/api/v1/investments/transactions/" + strconv.FormatInt(original.Transaction.ID, 10)
	replace := investmentReinvestmentReplacementRequest{Reason: "broker restated", Replacement: reinvestment(4, 4000)}
	previewRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, base+"/replace-reinvested-dividend/reconciliation-impact", replace, http.StatusOK)
	var preview reconciliationImpactResponse
	require.NoError(t, json.NewDecoder(previewRes.Body).Decode(&preview))
	require.NotNil(t, preview.GainImpact)
	require.Len(t, preview.GainImpact.Changes, 1)
	change := preview.GainImpact.Changes[0]
	assert.Equal(t, "revised", change.ChangeKind)
	assertGainCoefficient(t, 14000, 2, change.Before.RealizedGainValue, change.Before.RealizedGainScale)
	assertGainCoefficient(t, 9000, 2, change.After.RealizedGainValue, change.After.RealizedGainScale)

	expectInvestmentConflict(t, handler, f, base+"/replace-reinvested-dividend", replace, "INVESTMENT_GAIN_IMPACT_ACKNOWLEDGEMENT_REQUIRED")
	replace.GainImpactAcknowledgement = "stale"
	expectInvestmentConflict(t, handler, f, base+"/replace-reinvested-dividend", replace, "INVESTMENT_GAIN_IMPACT_ACKNOWLEDGEMENT_STALE")
	replace.GainImpactAcknowledgement = preview.GainImpact.Acknowledgement
	replacedRes := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, base+"/replace-reinvested-dividend", replace, http.StatusCreated)
	var replaced investmentBuyReplacementResponse
	require.NoError(t, json.NewDecoder(replacedRes.Body).Decode(&replaced))
	require.NotNil(t, replaced.Replacement.LotID)
	expectInvestmentConflict(t, handler, f, base+"/reverse-reinvested-dividend",
		investmentSaleReversalRequest{Reason: "again"}, "INVESTMENT_REINVESTMENT_ALREADY_CORRECTED")

	// Reversing the replacement replays the sale onto the bought shares.
	replacementBase := "/api/v1/investments/transactions/" + strconv.FormatInt(replaced.Replacement.Transaction.ID, 10)
	reverse := investmentSaleReversalRequest{Reason: "the fund paid cash"}
	reverseImpact := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, replacementBase+"/reverse-reinvested-dividend/reconciliation-impact", reverse, http.StatusOK)
	var reversePreview reconciliationImpactResponse
	require.NoError(t, json.NewDecoder(reverseImpact.Body).Decode(&reversePreview))
	reverse.GainImpactAcknowledgement = reversePreview.GainImpact.Acknowledgement
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, replacementBase+"/reverse-reinvested-dividend", reverse, http.StatusCreated)
	// A buy is not a reinvestment, and a reinvestment is not a dividend.
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, replacementBase+"/reverse-dividend/reconciliation-impact",
		investmentSaleReversalRequest{Reason: "wrong family"}, http.StatusNotFound)
}
