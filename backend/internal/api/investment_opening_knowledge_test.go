package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/app"
)

func TestLotResponseSeparatesOriginalAndProjectedBasisKnowledge(t *testing.T) {
	for _, test := range []struct {
		name, opening, projected string
		original, remaining      any
	}{
		{"unknown original, known projection", "unknown", "known", nil, "1250"},
		{"known zero original, unknown projection", "known", "unknown", "0", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(toInvestmentLotResponse(app.InvestmentLot{
				OpeningBasisKnowledge: test.opening, BasisKnowledge: test.projected,
				CostBasisValue: 0, CostBasisScale: 0,
				RemainingCostBasisValue: 1250, RemainingCostBasisScale: 2,
				MetadataJSON: "{}",
			}))
			require.NoError(t, err)
			var response map[string]any
			require.NoError(t, json.Unmarshal(encoded, &response))
			require.Equal(t, test.original, response["cost_basis_value"])
			require.Equal(t, test.remaining, response["remaining_cost_basis_value"])
			require.Equal(t, test.opening, response["opening_basis_knowledge"])
			require.Equal(t, test.projected, response["basis_knowledge"])
			if test.original == nil {
				require.Nil(t, response["cost_basis_scale"])
			}
			if test.remaining == nil {
				require.Nil(t, response["remaining_cost_basis_scale"])
			}
		})
	}
}
