package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnknownProjectedBasisAPIKeepsQuantityAndReturnsNullBasis(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "NULLBASIS")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy",
		tradeRequestBody(f, holding.ID, instrument.CommodityID, "2", 2000), http.StatusCreated)
	_, err := database.Exec(`UPDATE investment_lot_state SET basis_knowledge = 'unknown',
		remaining_cost_basis_value = NULL, remaining_cost_basis_scale = NULL`)
	require.NoError(t, err)
	for _, test := range []struct{ path, key string }{
		{"/api/v1/investments/lots", "lots"},
		{"/api/v1/investments/positions", "positions"},
		{"/api/v1/investments/gains", "unrealized"},
	} {
		t.Run(test.key, func(t *testing.T) {
			result := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, test.path, nil, http.StatusOK)
			var body map[string]json.RawMessage
			require.NoError(t, json.NewDecoder(result.Body).Decode(&body))
			var rows []map[string]any
			require.NoError(t, json.Unmarshal(body[test.key], &rows))
			require.Len(t, rows, 1)
			require.Equal(t, "unknown", rows[0]["basis_knowledge"])
			for _, key := range []string{"remaining_cost_basis_value", "remaining_cost_basis_scale"} {
				require.Contains(t, rows[0], key, "NULL is explicit, not an omitted field")
				require.Nil(t, rows[0][key])
			}
			if test.key == "lots" {
				require.Equal(t, "2", rows[0]["remaining_quantity_value"])
			} else {
				require.Equal(t, "2", rows[0]["quantity_value"])
			}
			if test.key == "unrealized" {
				require.Equal(t, "unknown_basis", rows[0]["gain_unavailable"])
				require.NotContains(t, rows[0], "unrealized_gain_value")
				require.Contains(t, rows[0], "market_value_value")
			}
		})
	}
}
