package api

import (
	"encoding/json"
	"maps"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPaymentAttributionAPIEntryOmissionNullAndReplacementPresence(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "ATTR")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	expense := createLedgerAccount(t, handler, f.sessionCookie, f.csrfToken, "Borrow costs", "expense", "expense", f.commodityID, 2)
	financial := func() map[string]any {
		return map[string]any{"transaction_date": "2026-03-05", "cash_account_id": f.cashAccount.ID,
			"cash_commodity_id": f.commodityID, "expense_account_id": expense.ID, "amount_value": "500", "amount_scale": 2}
	}
	post := func(path string, body any, status int) *http.Response {
		return doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, body, status).Result()
	}
	entry := post("/api/v1/investments/payment-in-lieu", financial(), http.StatusCreated)
	var created transactionResponse
	require.NoError(t, json.NewDecoder(entry.Body).Decode(&created))
	current := created.ID
	readTerms := func() *investmentCorrectionPaymentInLieuTerms {
		res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodGet, "/api/v1/investments/transactions/"+strconv.FormatInt(created.ID, 10)+"/correction-chain", nil, http.StatusOK)
		var chain investmentCorrectionChainResponse
		require.NoError(t, json.NewDecoder(res.Body).Decode(&chain))
		require.NotNil(t, chain.EffectivePaymentInLieu)
		return chain.EffectivePaymentInLieu
	}
	terms := readTerms()
	require.Nil(t, terms.HoldingAccountID)
	require.Nil(t, terms.CommodityID)
	require.Nil(t, terms.ExDate)
	require.Equal(t, "incomplete", terms.Attribution.Status)
	for _, step := range []struct {
		name                string
		fields              map[string]any
		holding, instrument bool
		date                *string
	}{
		{"assign", map[string]any{"holding_account_id": holding.ID, "commodity_id": instrument.CommodityID, "ex_date": "2026-03-01"}, true, true, new("2026-03-01")},
		{"omit preserves", nil, true, true, new("2026-03-01")},
		{"null clears instrument", map[string]any{"commodity_id": nil}, true, false, new("2026-03-01")},
		{"clear all", map[string]any{"holding_account_id": nil, "ex_date": nil}, false, false, nil},
	} {
		body := financial()
		maps.Copy(body, step.fields)
		res := post("/api/v1/investments/transactions/"+strconv.FormatInt(current, 10)+"/replace-payment-in-lieu", map[string]any{"reason": step.name, "replacement": body}, http.StatusCreated)
		var replacement investmentDividendReplacementResponse
		require.NoError(t, json.NewDecoder(res.Body).Decode(&replacement))
		current = replacement.ReplacementTransaction.ID
		terms = readTerms()
		require.Equal(t, step.holding, terms.HoldingAccountID != nil)
		require.Equal(t, step.instrument, terms.CommodityID != nil)
		require.Equal(t, step.date, terms.ExDate)
	}
	for key, value := range map[string]any{"holding_account_id": f.cashAccount.ID, "commodity_id": f.commodityID, "ex_date": "2026-02-31"} {
		bad := financial()
		bad[key] = value
		post("/api/v1/investments/payment-in-lieu", bad, http.StatusBadRequest)
	}
}
