package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// A sale over unknown basis reports NULL basis and gain with explicit
// knowledge, and its currency's realized total is NULL rather than the sum of
// the known entries (T-145).
func TestUnresolvedSaleReportsNullBasisGainAndTotal(t *testing.T) {
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "UNK")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", tradeRequestBody(f, holding.ID, instrument.CommodityID, "5", 50000), http.StatusCreated)
	// The projection alone is enough for the sale writer to see unknown basis.
	_, err := database.Exec(`UPDATE investment_lot_state SET basis_knowledge = 'unknown',
		remaining_cost_basis_value = NULL, remaining_cost_basis_scale = NULL`)
	require.NoError(t, err)

	sale := tradeRequestBody(f, holding.ID, instrument.CommodityID, "2", 24000)
	sale.TransactionDate = "2026-03-01"
	res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost, "/api/v1/investments/sell/preview", sale, http.StatusOK)
	var preview map[string]any
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &preview))
	require.Equal(t, "unknown", preview["basis_knowledge"])
	require.Contains(t, preview, "realized_gain")
	require.Nil(t, preview["realized_gain"])
	require.Nil(t, preview["realized_gain_scale"])
	decision := preview["disposal_decision"].(map[string]any)
	require.Equal(t, "unknown", decision["basis_knowledge"])
	require.Nil(t, decision["disposed_basis_value"])
	allocation := decision["allocations"].([]any)[0].(map[string]any)
	require.Equal(t, "unknown", allocation["basis_knowledge"])
	require.Nil(t, allocation["cost_basis_value"])
	require.Equal(t, "2", allocation["quantity_value"], "quantity is exact whatever the basis")

	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/sell", sale, http.StatusCreated)
	res = doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/investments/gains", nil, http.StatusOK)
	var gains map[string]any
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &gains))
	realized := gains["realized"].([]any)
	require.Len(t, realized, 1)
	entry := realized[0].(map[string]any)
	require.Equal(t, "unknown", entry["basis_knowledge"])
	require.Nil(t, entry["realized_gain_value"])
	require.Nil(t, entry["disposed_basis_value"])
	require.Equal(t, "24000", entry["proceeds_value"])
	total := gains["realized_totals"].([]any)[0].(map[string]any)
	require.Equal(t, "unknown", total["basis_knowledge"])
	require.Nil(t, total["total_gain_value"])
	require.Equal(t, float64(1), total["unresolved_count"])
}
