package api

import (
	"encoding/json"
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
