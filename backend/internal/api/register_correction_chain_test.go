package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T-120 #135: the register response names each row's correction chain.
func TestRegisterResponseCarriesCorrectionChain(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "ICHAINREG")
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
	res = doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/transactions/"+strconv.FormatInt(split.Transaction.ID, 10)+"/replace-split",
		investmentSplitReplacementRequest{Reason: "it was 3-for-1", EffectiveOn: "2026-02-01",
			RatioNumerator: 3, RatioDenominator: 1}, http.StatusCreated)
	var replaced investmentSplitReplacementResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&replaced))

	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet,
		"/api/v1/accounts/"+strconv.FormatInt(holding.ID, 10)+"/register", nil, http.StatusOK)
	var register struct {
		Entries []struct {
			TransactionID   int64 `json:"transaction_id"`
			CorrectionChain *struct {
				RootTransactionID      int64  `json:"root_transaction_id"`
				Role                   string `json:"role"`
				EffectiveTransactionID *int64 `json:"effective_transaction_id"`
				NetEffect              struct {
					QuantityValue string `json:"quantity_value"`
					QuantityScale int    `json:"quantity_scale"`
				} `json:"net_effect"`
				Members []struct {
					TransactionID int64  `json:"transaction_id"`
					Role          string `json:"role"`
					Reason        string `json:"reason"`
				} `json:"members"`
			} `json:"correction_chain"`
		} `json:"entries"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&register))
	chained := 0
	for _, entry := range register.Entries {
		if entry.CorrectionChain == nil {
			continue
		}
		chained++
		chain := entry.CorrectionChain
		assert.Equal(t, split.Transaction.ID, chain.RootTransactionID)
		require.Len(t, chain.Members, 3)
		assert.Equal(t, []string{"original", "reversal", "replacement"},
			[]string{chain.Members[0].Role, chain.Members[1].Role, chain.Members[2].Role})
		assert.Equal(t, "it was 3-for-1", chain.Members[2].Reason)
		require.NotNil(t, chain.EffectiveTransactionID)
		assert.Equal(t, replaced.Replacement.ID, *chain.EffectiveTransactionID)
		assert.Equal(t, "20", chain.NetEffect.QuantityValue, "the 3-for-1 split adds 20 shares, counted once")
	}
	assert.Equal(t, 3, chained, "the buy is not part of the split's chain")
}
