package api

import (
	"encoding/json"
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
