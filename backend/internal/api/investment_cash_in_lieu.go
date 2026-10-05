package api

import (
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/exact"
)

type cashInLieuRequest struct {
	SplitTransactionID        int64                            `json:"split_transaction_id"`
	DisposalOn                string                           `json:"disposal_on"`
	PaymentOn                 string                           `json:"payment_on"`
	QuantityValue             exact.Coefficient                `json:"quantity_value"`
	QuantityScale             int                              `json:"quantity_scale"`
	CashAccountID             int64                            `json:"cash_account_id"`
	CurrencyID                int64                            `json:"currency_id"`
	ProceedsValue             moneyCoefficient                 `json:"proceeds_value"`
	ProceedsScale             int                              `json:"proceeds_scale"`
	CostBasisMethod           string                           `json:"cost_basis_method,omitempty"`
	LotAllocations            []investmentLotAllocationRequest `json:"lot_allocations,omitempty"`
	Memo                      string                           `json:"memo"`
	ChangeReason              string                           `json:"change_reason"`
	ReconciliationOverride    bool                             `json:"reconciliation_override"`
	GainImpactAcknowledgement string                           `json:"gain_impact_acknowledgement,omitempty"`
}

type cashInLieuPreviewResponse struct {
	Allocations []investmentLotDisposalResponse `json:"allocations"`
	Impact      reconciliationImpactResponse    `json:"impact"`
}

func cashInLieuInput(owner app.Owner, r *http.Request, request cashInLieuRequest) app.CashInLieuInput {
	allocations := make([]app.InvestmentLotAllocationInput, 0, len(request.LotAllocations))
	for _, allocation := range request.LotAllocations {
		allocations = append(allocations, app.InvestmentLotAllocationInput{LotID: allocation.LotID,
			QuantityValue: allocation.QuantityValue, QuantityScale: allocation.QuantityScale})
	}
	return app.CashInLieuInput{
		OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
		SplitTransactionID: request.SplitTransactionID, DisposalOn: request.DisposalOn, PaymentOn: request.PaymentOn,
		QuantityValue: request.QuantityValue, QuantityScale: request.QuantityScale,
		CashAccountID: request.CashAccountID, CurrencyID: request.CurrencyID,
		ProceedsValue: int64(request.ProceedsValue), ProceedsScale: request.ProceedsScale,
		CostBasisMethod: request.CostBasisMethod, LotAllocations: allocations,
		Memo: request.Memo, ChangeReason: request.ChangeReason,
		ReconciliationOverride:    request.ReconciliationOverride,
		GainImpactAcknowledgement: request.GainImpactAcknowledgement,
	}
}

func cashInLieu(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request cashInLieuRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		result, err := investmentService.CashInLieu(r.Context(), cashInLieuInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "cash in lieu", err)
			return
		}
		writeJSON(w, http.StatusCreated, toInvestmentTradeResponse(result))
	}))
}

func cashInLieuPreview(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		var request cashInLieuRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		result, impact, err := investmentService.PreviewCashInLieu(r.Context(), cashInLieuInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "cash in lieu preview", err)
			return
		}
		response := toReconciliationImpactResponse(impact)
		gainImpact, err := toGainImpactResponse(impact.GainImpact)
		if err != nil {
			writeAPIError(w, http.StatusUnprocessableEntity, "LEDGER_OVERFLOW", "gain impact value exceeds the coefficient range")
			return
		}
		response.GainImpact = gainImpact
		writeJSON(w, http.StatusOK, cashInLieuPreviewResponse{
			Allocations: toInvestmentLotDisposalResponses(result.Allocations), Impact: response,
		})
	}
}
