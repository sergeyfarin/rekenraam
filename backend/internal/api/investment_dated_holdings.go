package api

import (
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/exact"
)

type datedHoldingResponse struct {
	AccountID               int64                     `json:"account_id"`
	CommodityID             int64                     `json:"commodity_id"`
	CostCommodityID         int64                     `json:"cost_commodity_id"`
	PositionSide            string                    `json:"position_side"`
	QuantityValue           exact.Coefficient         `json:"quantity_value"`
	QuantityScale           int                       `json:"quantity_scale"`
	TransferBasisAllocation string                    `json:"transfer_basis_allocation"`
	Lots                    []datedHoldingLotResponse `json:"lots"`
}

type datedHoldingLotResponse struct {
	LotID                   int64             `json:"lot_id"`
	OpenedOn                string            `json:"opened_on"`
	QuantityValue           exact.Coefficient `json:"quantity_value"`
	QuantityScale           int               `json:"quantity_scale"`
	BasisKnowledge          string            `json:"basis_knowledge"`
	RemainingCostBasisValue *moneyCoefficient `json:"remaining_cost_basis_value"`
	RemainingCostBasisScale *int              `json:"remaining_cost_basis_scale"`
}

type datedHoldingsResponse struct {
	AsOf     string                 `json:"as_of"`
	Holdings []datedHoldingResponse `json:"holdings"`
}

// datedHoldings serves the historical-entry selectors (#166): one request per
// selector, composed by the backend, never a per-lot fan-out.
func datedHoldings(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		asOf := r.URL.Query().Get("as_of")
		holdings, err := investmentService.DatedHoldings(r.Context(), owner.ID, asOf)
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "read dated holdings", err)
			return
		}
		response := datedHoldingsResponse{AsOf: asOf, Holdings: make([]datedHoldingResponse, 0, len(holdings))}
		for _, holding := range holdings {
			lots := make([]datedHoldingLotResponse, 0, len(holding.Lots))
			for _, lot := range holding.Lots {
				lots = append(lots, datedHoldingLotResponse{LotID: lot.LotID, OpenedOn: lot.OpenedOn,
					QuantityValue: lot.QuantityValue, QuantityScale: lot.QuantityScale,
					BasisKnowledge:          responseKnowledge(lot.BasisKnowledge),
					RemainingCostBasisValue: projectedBasisValue(lot.RemainingCostBasisValue, lot.BasisKnowledge),
					RemainingCostBasisScale: projectedBasisScale(lot.RemainingCostBasisScale, lot.BasisKnowledge)})
			}
			response.Holdings = append(response.Holdings, datedHoldingResponse{
				AccountID: holding.AccountID, CommodityID: holding.CommodityID, CostCommodityID: holding.CostCommodityID,
				PositionSide: "long", QuantityValue: holding.QuantityValue, QuantityScale: holding.QuantityScale,
				TransferBasisAllocation: holding.TransferBasisAllocation, Lots: lots,
			})
		}
		writeJSON(w, http.StatusOK, response)
	}
}
