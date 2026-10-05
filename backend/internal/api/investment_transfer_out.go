package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/exact"
)

type externalTransferOutRequest struct {
	EffectiveOn     string                           `json:"effective_on"`
	SourceAccountID int64                            `json:"source_account_id"`
	CommodityID     int64                            `json:"commodity_id"`
	CostCommodityID int64                            `json:"cost_commodity_id"`
	LotAllocations  []investmentLotAllocationRequest `json:"lot_allocations"`
	// QuantityValue/Scale move a total out of an average-cost pool instead of
	// selected lots.
	QuantityValue             exact.Coefficient `json:"quantity_value,omitempty"`
	QuantityScale             int               `json:"quantity_scale,omitempty"`
	SourceEvidence            json.RawMessage   `json:"source_evidence,omitempty"`
	Memo                      string            `json:"memo"`
	ChangeReason              string            `json:"change_reason"`
	ReconciliationOverride    bool              `json:"reconciliation_override"`
	GainImpactAcknowledgement string            `json:"gain_impact_acknowledgement,omitempty"`
}

type externalTransferOutLinkResponse struct {
	SourceLotID           int64             `json:"source_lot_id"`
	QuantityValue         exact.Coefficient `json:"quantity_value"`
	QuantityScale         int               `json:"quantity_scale"`
	CarriedBasisValue     moneyCoefficient  `json:"carried_basis_value"`
	CarriedBasisScale     int               `json:"carried_basis_scale"`
	OriginalDateKnowledge string            `json:"original_date_knowledge"`
	OriginalAcquiredOn    *string           `json:"original_acquired_on"`
}

type externalTransferOutPlanResponse struct {
	BasisAllocation string                            `json:"basis_allocation"`
	CostBasisMethod string                            `json:"cost_basis_method"`
	ResolutionTier  string                            `json:"resolution_tier"`
	Links           []externalTransferOutLinkResponse `json:"links"`
	BasisValue      moneyCoefficient                  `json:"basis_value"`
	BasisScale      int                               `json:"basis_scale"`
}

type externalTransferOutResponse struct {
	Transaction transactionResponse             `json:"transaction"`
	Plan        externalTransferOutPlanResponse `json:"plan"`
}

type externalTransferOutPreviewResponse struct {
	Plan   externalTransferOutPlanResponse `json:"plan"`
	Impact reconciliationImpactResponse    `json:"impact"`
}

func externalTransferOutInput(owner app.Owner, r *http.Request, request externalTransferOutRequest) app.ExternalTransferOutInput {
	allocations := make([]app.InvestmentLotAllocationInput, 0, len(request.LotAllocations))
	for _, allocation := range request.LotAllocations {
		allocations = append(allocations, app.InvestmentLotAllocationInput{
			LotID: allocation.LotID, QuantityValue: allocation.QuantityValue,
			QuantityScale: allocation.QuantityScale,
		})
	}
	evidence := rawJSONText(request.SourceEvidence)
	if evidence == "null" {
		evidence = ""
	}
	return app.ExternalTransferOutInput{
		OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r),
		RequestID: RequestIDFromContext(r.Context()), EffectiveOn: request.EffectiveOn,
		SourceAccountID: request.SourceAccountID, CommodityID: request.CommodityID,
		CostCommodityID: request.CostCommodityID, Allocations: allocations,
		QuantityValue: request.QuantityValue, QuantityScale: request.QuantityScale,
		SourceEvidenceJSON: evidence, Memo: request.Memo, ChangeReason: request.ChangeReason,
		ReconciliationOverride:    request.ReconciliationOverride,
		GainImpactAcknowledgement: request.GainImpactAcknowledgement,
	}
}

func toExternalTransferOutPlanResponse(plan app.ExternalTransferOutPlan) externalTransferOutPlanResponse {
	out := externalTransferOutPlanResponse{BasisAllocation: plan.BasisAllocation, CostBasisMethod: plan.CostBasisMethod,
		ResolutionTier: plan.ResolutionTier, BasisValue: moneyCoefficient(plan.BasisValue), BasisScale: plan.BasisScale,
		Links: make([]externalTransferOutLinkResponse, 0, len(plan.Links))}
	for _, link := range plan.Links {
		response := externalTransferOutLinkResponse{SourceLotID: link.SourceLotID,
			QuantityValue: link.QuantityValue, QuantityScale: link.QuantityScale,
			CarriedBasisValue: moneyCoefficient(link.CarriedBasisValue), CarriedBasisScale: link.CarriedBasisScale,
			OriginalDateKnowledge: link.OriginalDateKnowledge}
		if link.OriginalAcquiredOn != "" {
			date := link.OriginalAcquiredOn
			response.OriginalAcquiredOn = &date
		}
		out.Links = append(out.Links, response)
	}
	return out
}

func externalTransferOut(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request externalTransferOutRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		result, err := investmentService.ExternalTransferOut(r.Context(), externalTransferOutInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "outbound investment transfer", err)
			return
		}
		writeJSON(w, http.StatusCreated, externalTransferOutResponse{
			Transaction: toTransactionResponse(result.Transaction), Plan: toExternalTransferOutPlanResponse(result.Plan),
		})
	}))
}

func externalTransferOutPreview(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		var request externalTransferOutRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		preview, err := investmentService.PreviewExternalTransferOut(r.Context(), externalTransferOutInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "outbound investment transfer preview", err)
			return
		}
		impact := toReconciliationImpactResponse(preview.Impact)
		gainImpact, err := toGainImpactResponse(preview.Impact.GainImpact)
		if err != nil {
			writeAPIError(w, http.StatusUnprocessableEntity, "LEDGER_OVERFLOW", "gain impact value exceeds the coefficient range")
			return
		}
		impact.GainImpact = gainImpact
		writeJSON(w, http.StatusOK, externalTransferOutPreviewResponse{
			Plan: toExternalTransferOutPlanResponse(preview.Plan), Impact: impact,
		})
	}
}

func externalTransferOutReconciliationImpact(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		var request externalTransferOutRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		impact, err := investmentService.PreviewExternalTransferOutReconciliationImpact(r.Context(), externalTransferOutInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "outbound investment transfer impact", err)
			return
		}
		writeReconciliationImpact(w, impact)
	}
}
