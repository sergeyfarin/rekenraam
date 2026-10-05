package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/exact"
)

type capitalReturnRequest struct {
	HoldingAccountID          int64             `json:"holding_account_id"`
	CommodityID               int64             `json:"commodity_id"`
	CashAccountID             int64             `json:"cash_account_id"`
	CurrencyID                int64             `json:"currency_id"`
	EffectiveOn               string            `json:"effective_on"`
	PaymentOn                 string            `json:"payment_on"`
	AmountValue               exact.Coefficient `json:"amount_value"`
	AmountScale               int               `json:"amount_scale"`
	SourceEvidence            json.RawMessage   `json:"source_evidence,omitempty"`
	Memo                      string            `json:"memo"`
	ChangeReason              string            `json:"change_reason"`
	ReconciliationOverride    bool              `json:"reconciliation_override"`
	GainImpactAcknowledgement string            `json:"gain_impact_acknowledgement,omitempty"`
}

type capitalReturnEffectResponse struct {
	LotID                 int64             `json:"lot_id"`
	EntitledQuantityValue exact.Coefficient `json:"entitled_quantity_value"`
	EntitledQuantityScale int               `json:"entitled_quantity_scale"`
	AllocatedValue        exact.Coefficient `json:"allocated_value"`
	AllocatedScale        int               `json:"allocated_scale"`
	ReductionValue        exact.Coefficient `json:"reduction_value"`
	ReductionScale        int               `json:"reduction_scale"`
	ExcessValue           exact.Coefficient `json:"excess_value"`
	ExcessScale           int               `json:"excess_scale"`
}

type capitalReturnResponse struct {
	Transaction transactionResponse           `json:"transaction"`
	Effects     []capitalReturnEffectResponse `json:"effects"`
}

type capitalReturnPreviewResponse struct {
	Effects []capitalReturnEffectResponse `json:"effects"`
	Impact  reconciliationImpactResponse  `json:"impact"`
}

func capitalReturnInput(owner app.Owner, r *http.Request, request capitalReturnRequest) app.CapitalReturnInput {
	evidence := rawJSONText(request.SourceEvidence)
	if evidence == "null" {
		evidence = ""
	}
	return app.CapitalReturnInput{
		OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
		HoldingAccountID: request.HoldingAccountID, CommodityID: request.CommodityID,
		CashAccountID: request.CashAccountID, CurrencyID: request.CurrencyID,
		EffectiveOn: request.EffectiveOn, PaymentOn: request.PaymentOn,
		AmountValue: request.AmountValue, AmountScale: request.AmountScale,
		SourceEvidenceJSON: evidence, Memo: request.Memo, ChangeReason: request.ChangeReason,
		ReconciliationOverride:    request.ReconciliationOverride,
		GainImpactAcknowledgement: request.GainImpactAcknowledgement,
	}
}

func toCapitalReturnEffectResponses(effects []app.CapitalReturnEffect) []capitalReturnEffectResponse {
	out := make([]capitalReturnEffectResponse, 0, len(effects))
	for _, effect := range effects {
		out = append(out, capitalReturnEffectResponse(effect))
	}
	return out
}

func capitalReturn(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request capitalReturnRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		result, err := investmentService.CapitalReturn(r.Context(), capitalReturnInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "return of capital", err)
			return
		}
		writeJSON(w, http.StatusCreated, capitalReturnResponse{
			Transaction: toTransactionResponse(result.Transaction), Effects: toCapitalReturnEffectResponses(result.Effects),
		})
	}))
}

func capitalReturnPreview(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		var request capitalReturnRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		preview, err := investmentService.PreviewCapitalReturn(r.Context(), capitalReturnInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "return of capital preview", err)
			return
		}
		impact := toReconciliationImpactResponse(preview.Impact)
		gainImpact, err := toGainImpactResponse(preview.Impact.GainImpact)
		if err != nil {
			writeAPIError(w, http.StatusUnprocessableEntity, "LEDGER_OVERFLOW", "gain impact value exceeds the coefficient range")
			return
		}
		impact.GainImpact = gainImpact
		writeJSON(w, http.StatusOK, capitalReturnPreviewResponse{
			Effects: toCapitalReturnEffectResponses(preview.Effects), Impact: impact,
		})
	}
}
