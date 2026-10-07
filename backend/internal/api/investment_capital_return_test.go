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

func TestCapitalReturnAPIPreviewMatchesCommitAndNamesNoHoldings(t *testing.T) {
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "IROC")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	path := "/api/v1/investments/return-of-capital"
	request := capitalReturnRequest{HoldingAccountID: holding.ID, CommodityID: instrument.CommodityID,
		CashAccountID: f.cashAccount.ID, CurrencyID: f.commodityID, EffectiveOn: "2026-02-01", PaymentOn: "2026-02-10",
		AmountValue: exact.New(1000), AmountScale: 2, SourceEvidence: json.RawMessage(`{"notice":"42"}`)}
	refused := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, request, http.StatusConflict)
	assert.Contains(t, refused.Body.String(), "INVESTMENT_CAPITAL_RETURN_NO_HOLDINGS")

	buy := tradeRequestBody(f, holding.ID, instrument.CommodityID, "1", 700)
	buy.TransactionDate = "2026-01-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy", buy, http.StatusCreated)
	res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, path+"/preview", request, http.StatusOK)
	var preview capitalReturnPreviewResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&preview))
	require.Len(t, preview.Effects, 1)
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, path, request, http.StatusForbidden)
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, request, http.StatusCreated)
	var posted capitalReturnResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&posted))
	assert.Equal(t, preview.Effects, posted.Effects, "preview equals commit")
	assert.Equal(t, "2026-02-10", posted.Transaction.TransactionDate, "cash posts on the payment date")
	assert.Zero(t, exact.ScaledIntFromCoefficient(posted.Effects[0].ExcessValue, posted.Effects[0].ExcessScale).Cmp(
		exact.ScaledIntFromInt64(300, 2)), "10.00 on a 7.00 lot leaves 3.00 unresolved")
}

func TestCapitalReturnReplacementAPIPreviewMatchesCommitAndFencesKind(t *testing.T) {
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "RCOR")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	buy := tradeRequestBody(f, holding.ID, instrument.CommodityID, "2", 2000)
	buy.TransactionDate = "2026-01-01"
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy", buy, http.StatusCreated)
	var bought investmentTradeResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&bought))
	request := capitalReturnRequest{HoldingAccountID: holding.ID, CommodityID: instrument.CommodityID,
		CashAccountID: f.cashAccount.ID, CurrencyID: f.commodityID, EffectiveOn: "2026-02-01", PaymentOn: "2026-02-10",
		AmountValue: exact.New(400), AmountScale: 2}
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/return-of-capital", request, http.StatusCreated)
	var posted capitalReturnResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&posted))
	path := fmt.Sprintf("/api/v1/investments/transactions/%d/replace-return-of-capital", posted.Transaction.ID)
	request.AmountValue = exact.New(1500)
	request.LotEntitlements = []capitalReturnEntitlementRequest{{LotID: *bought.LotID, QuantityValue: exact.New(1)}}
	correction := capitalReturnReplacementRequest{Reason: "corporate action notice corrected", Replacement: request}
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, path+"/preview", correction, http.StatusOK)
	var preview capitalReturnPreviewResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&preview))
	require.Len(t, preview.Effects, 1)
	doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, path, correction, http.StatusForbidden)
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, correction, http.StatusCreated)
	var replaced capitalReturnReplacementResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&replaced))
	require.Equal(t, preview.Effects, replaced.Replacement.Effects)
	require.Equal(t, posted.Transaction.ID, replaced.CorrectedTransactionID)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, correction, http.StatusConflict)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		fmt.Sprintf("/api/v1/investments/transactions/%d/replace-return-of-capital", bought.Transaction.ID), correction, http.StatusNotFound)
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet,
		fmt.Sprintf("/api/v1/investments/transactions/%d/correction-chain", posted.Transaction.ID), nil, http.StatusOK)
	var chain investmentCorrectionChainResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&chain))
	require.True(t, chain.CanReplaceCapitalReturn)
	require.Equal(t, replaced.Replacement.Transaction.ID, *chain.EffectiveTransactionID)
	require.Equal(t, request.LotEntitlements, chain.EffectiveCapitalReturn.LotEntitlements)
}
