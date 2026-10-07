package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/exact"
)

type capitalReturnRequest struct {
	HoldingAccountID          int64                             `json:"holding_account_id"`
	CommodityID               int64                             `json:"commodity_id"`
	CashAccountID             int64                             `json:"cash_account_id"`
	CurrencyID                int64                             `json:"currency_id"`
	EffectiveOn               string                            `json:"effective_on"`
	PaymentOn                 string                            `json:"payment_on"`
	AmountValue               exact.Coefficient                 `json:"amount_value"`
	AmountScale               int                               `json:"amount_scale"`
	SourceEvidence            json.RawMessage                   `json:"source_evidence,omitempty"`
	Memo                      string                            `json:"memo"`
	ChangeReason              string                            `json:"change_reason"`
	ReconciliationOverride    bool                              `json:"reconciliation_override"`
	GainImpactAcknowledgement string                            `json:"gain_impact_acknowledgement,omitempty"`
	LotEntitlements           []capitalReturnEntitlementRequest `json:"lot_entitlements,omitempty"`
	EntitledLotIDs            []int64                           `json:"entitled_lot_ids,omitempty"`
}

type capitalReturnEntitlementRequest struct {
	LotID         int64             `json:"lot_id"`
	QuantityValue exact.Coefficient `json:"quantity_value"`
	QuantityScale int               `json:"quantity_scale"`
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
	var entitlements []app.CapitalReturnEntitlement
	if request.LotEntitlements != nil {
		entitlements = make([]app.CapitalReturnEntitlement, 0, len(request.LotEntitlements))
	}
	for _, e := range request.LotEntitlements {
		entitlements = append(entitlements, app.CapitalReturnEntitlement(e))
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
		EntitledLotIDs:            request.EntitledLotIDs, LotEntitlements: entitlements,
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

func reverseCapitalReturn(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, request, ok := correctionRoute[investmentSaleReversalRequest](w, r)
		if !ok {
			return
		}
		transaction, err := investmentService.ReverseCapitalReturn(r.Context(), app.ReverseInvestmentCapitalReturnInput{
			OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
			TransactionID: transactionID, Reason: request.Reason, ReconciliationOverride: request.ReconciliationOverride,
			GainImpactAcknowledgement: request.GainImpactAcknowledgement,
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "reverse return of capital", err)
			return
		}
		writeJSON(w, http.StatusCreated, investmentSaleReversalResponse{Transaction: toTransactionResponse(transaction), CorrectedTransactionID: transactionID})
	}))
}

func reverseCapitalReturnReconciliationImpact(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		transactionID, request, ok := correctionRoute[investmentSaleReversalRequest](w, r)
		if !ok {
			return
		}
		impact, err := investmentService.ReverseCapitalReturnReconciliationImpact(r.Context(), app.ReverseInvestmentCapitalReturnInput{
			OwnerUserID: owner.ID, TransactionID: transactionID, Reason: request.Reason,
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "preview return of capital reversal", err)
			return
		}
		writeReconciliationImpact(w, impact)
	}
}

func toCapitalReturnTerms(terms *app.CapitalReturnInput) *capitalReturnRequest {
	if terms == nil {
		return nil
	}
	entitlements := make([]capitalReturnEntitlementRequest, 0, len(terms.LotEntitlements))
	for _, e := range terms.LotEntitlements {
		entitlements = append(entitlements, capitalReturnEntitlementRequest(e))
	}
	return &capitalReturnRequest{HoldingAccountID: terms.HoldingAccountID, CommodityID: terms.CommodityID,
		CashAccountID: terms.CashAccountID, CurrencyID: terms.CurrencyID, EffectiveOn: terms.EffectiveOn, PaymentOn: terms.PaymentOn,
		AmountValue: terms.AmountValue, AmountScale: terms.AmountScale, SourceEvidence: json.RawMessage(terms.SourceEvidenceJSON),
		Memo: terms.Memo, EntitledLotIDs: terms.EntitledLotIDs, LotEntitlements: entitlements}
}

type capitalReturnReplacementRequest struct {
	Reason                    string               `json:"reason"`
	Replacement               capitalReturnRequest `json:"replacement"`
	ReconciliationOverride    bool                 `json:"reconciliation_override"`
	GainImpactAcknowledgement string               `json:"gain_impact_acknowledgement,omitempty"`
}

type capitalReturnReplacementResponse struct {
	Inverse                transactionResponse   `json:"inverse"`
	Replacement            capitalReturnResponse `json:"replacement"`
	CorrectedTransactionID int64                 `json:"corrected_transaction_id"`
}

func capitalReturnReplacementInput(owner app.Owner, r *http.Request, id int64, request capitalReturnReplacementRequest) app.ReplaceInvestmentCapitalReturnInput {
	return app.ReplaceInvestmentCapitalReturnInput{OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
		TransactionID: id, Reason: request.Reason, ReconciliationOverride: request.ReconciliationOverride, GainImpactAcknowledgement: request.GainImpactAcknowledgement,
		Replacement: capitalReturnInput(owner, r, request.Replacement)}
}

func replaceCapitalReturn(logger *slog.Logger, authService *app.AuthService, service *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		id, request, ok := correctionRoute[capitalReturnReplacementRequest](w, r)
		if !ok {
			return
		}
		result, err := service.ReplaceCapitalReturn(r.Context(), capitalReturnReplacementInput(owner, r, id, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "replace return of capital", err)
			return
		}
		writeJSON(w, http.StatusCreated, capitalReturnReplacementResponse{Inverse: toTransactionResponse(result.Inverse), Replacement: capitalReturnResponse{Transaction: toTransactionResponse(result.Replacement.Transaction), Effects: toCapitalReturnEffectResponses(result.Replacement.Effects)}, CorrectedTransactionID: result.CorrectedTransactionID})
	}))
}

func replaceCapitalReturnPreview(logger *slog.Logger, authService *app.AuthService, service *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		id, request, ok := correctionRoute[capitalReturnReplacementRequest](w, r)
		if !ok {
			return
		}
		result, err := service.PreviewCapitalReturnReplacement(r.Context(), capitalReturnReplacementInput(owner, r, id, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "preview return of capital replacement", err)
			return
		}
		impact := toReconciliationImpactResponse(result.Impact)
		impact.GainImpact, err = toGainImpactResponse(result.Impact.GainImpact)
		if err != nil {
			writeAPIError(w, http.StatusUnprocessableEntity, "LEDGER_OVERFLOW", "gain impact value exceeds the coefficient range")
			return
		}
		writeJSON(w, http.StatusOK, capitalReturnPreviewResponse{Effects: toCapitalReturnEffectResponses(result.Effects), Impact: impact})
	}
}
