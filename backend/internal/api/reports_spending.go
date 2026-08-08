package api

import (
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/exact"
)

type spendingGroupTotalResponse struct {
	CommodityID         int64             `json:"commodity_id"`
	QuantityValue       exact.Coefficient `json:"quantity_value"`
	QuantityScale       int               `json:"quantity_scale"`
	NormalQuantityValue exact.Coefficient `json:"normal_quantity_value"`
	ShareBasisPoints    int64             `json:"share_basis_points"`
}

type spendingGroupResponse struct {
	CategoryAccountID *int64                       `json:"category_account_id,omitempty"`
	PayeeID           *int64                       `json:"payee_id,omitempty"`
	PayeeLabel        string                       `json:"payee_label,omitempty"`
	Unassigned        bool                         `json:"unassigned"`
	Totals            []spendingGroupTotalResponse `json:"totals"`
}

type spendingQueryResponse struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	GroupBy   string `json:"group_by"`
	Direction string `json:"direction"`
}

type spendingResponse struct {
	StartDate           string                    `json:"start_date"`
	EndDate             string                    `json:"end_date"`
	GroupBy             string                    `json:"group_by"`
	Direction           string                    `json:"direction"`
	RankCommodityID     int64                     `json:"rank_commodity_id"`
	Query               spendingQueryResponse     `json:"query"`
	CommodityTotals     []balanceQuantityResponse `json:"commodity_totals"`
	Groups              []spendingGroupResponse   `json:"groups"`
	ExcludedSystemRoles []string                  `json:"excluded_system_roles"`
}

func spendingReport(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		result, err := transactionService.Spending(r.Context(), app.SpendingReportInput{
			StartDate: r.URL.Query().Get("start_date"),
			EndDate:   r.URL.Query().Get("end_date"),
			GroupBy:   r.URL.Query().Get("group_by"),
			Direction: r.URL.Query().Get("direction"),
		})
		if err != nil {
			writeLedgerServiceError(w, r, logger, "read spending report", err)
			return
		}

		groups := make([]spendingGroupResponse, 0, len(result.Groups))
		for _, group := range result.Groups {
			totals := make([]spendingGroupTotalResponse, 0, len(group.Totals))
			for _, total := range group.Totals {
				totals = append(totals, spendingGroupTotalResponse{
					CommodityID:         total.CommodityID,
					QuantityValue:       total.QuantityValue,
					QuantityScale:       total.QuantityScale,
					NormalQuantityValue: total.NormalQuantityValue,
					ShareBasisPoints:    total.ShareBasisPoints,
				})
			}
			groups = append(groups, spendingGroupResponse{
				CategoryAccountID: group.CategoryAccountID,
				PayeeID:           group.PayeeID,
				PayeeLabel:        group.PayeeLabel,
				Unassigned:        group.Unassigned,
				Totals:            totals,
			})
		}

		writeJSON(w, http.StatusOK, spendingResponse{
			StartDate:       result.StartDate,
			EndDate:         result.EndDate,
			GroupBy:         result.GroupBy,
			Direction:       result.Direction,
			RankCommodityID: result.RankCommodityID,
			Query: spendingQueryResponse{
				StartDate: result.StartDate,
				EndDate:   result.EndDate,
				GroupBy:   result.GroupBy,
				Direction: result.Direction,
			},
			CommodityTotals:     toBalanceQuantityResponses(result.CommodityTotals),
			Groups:              groups,
			ExcludedSystemRoles: result.ExcludedSystemRoles,
		})
	}
}
