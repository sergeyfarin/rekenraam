package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/exact"
)

// Share exchange (#177): the whole holding of one instrument in an account
// becomes another instrument at an exact ratio, carrying each lot's basis.
type shareExchangeRequest struct {
	EffectiveOn               string          `json:"effective_on"`
	HoldingAccountID          int64           `json:"holding_account_id"`
	DestinationHoldingID      int64           `json:"destination_holding_account_id,omitempty"`
	CommodityID               int64           `json:"commodity_id"`
	DestinationCommodityID    int64           `json:"destination_commodity_id"`
	RatioNumerator            int64           `json:"ratio_numerator"`
	RatioDenominator          int64           `json:"ratio_denominator"`
	SourceEvidence            json.RawMessage `json:"source_evidence,omitempty"`
	Memo                      string          `json:"memo"`
	ChangeReason              string          `json:"change_reason"`
	ReconciliationOverride    bool            `json:"reconciliation_override"`
	GainImpactAcknowledgement string          `json:"gain_impact_acknowledgement,omitempty"`
}

type shareExchangeLinkResponse struct {
	SourceLotID              int64             `json:"source_lot_id"`
	DestinationLotID         *int64            `json:"destination_lot_id"`
	CostCommodityID          int64             `json:"cost_commodity_id"`
	SourceQuantityValue      exact.Coefficient `json:"source_quantity_value"`
	SourceQuantityScale      int               `json:"source_quantity_scale"`
	DestinationQuantityValue exact.Coefficient `json:"destination_quantity_value"`
	DestinationQuantityScale int               `json:"destination_quantity_scale"`
	BasisKnowledge           string            `json:"basis_knowledge"`
	CarriedBasisValue        *string           `json:"carried_basis_value"`
	CarriedBasisScale        *int              `json:"carried_basis_scale"`
	OriginalDateKnowledge    string            `json:"original_date_knowledge"`
	OriginalAcquiredOn       *string           `json:"original_acquired_on"`
}

type shareExchangePlanResponse struct {
	RatioNumerator           int64                       `json:"ratio_numerator"`
	RatioDenominator         int64                       `json:"ratio_denominator"`
	Links                    []shareExchangeLinkResponse `json:"links"`
	SourceQuantityValue      exact.Coefficient           `json:"source_quantity_value"`
	SourceQuantityScale      int                         `json:"source_quantity_scale"`
	DestinationQuantityValue exact.Coefficient           `json:"destination_quantity_value"`
	DestinationQuantityScale int                         `json:"destination_quantity_scale"`
}

type shareExchangePreviewResponse struct {
	Plan   shareExchangePlanResponse    `json:"plan"`
	Impact reconciliationImpactResponse `json:"impact"`
}

type shareExchangeResponse struct {
	Transaction transactionResponse       `json:"transaction"`
	Plan        shareExchangePlanResponse `json:"plan"`
}

func shareExchangeInput(owner app.Owner, r *http.Request, request shareExchangeRequest) app.ShareExchangeInput {
	evidence := rawJSONText(request.SourceEvidence)
	if evidence == "null" {
		evidence = ""
	}
	return app.ShareExchangeInput{
		OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r),
		RequestID: RequestIDFromContext(r.Context()), EffectiveOn: request.EffectiveOn,
		HoldingAccountID: request.HoldingAccountID, DestinationHoldingAccountID: request.DestinationHoldingID,
		CommodityID: request.CommodityID, DestinationCommodityID: request.DestinationCommodityID,
		RatioNumerator: request.RatioNumerator, RatioDenominator: request.RatioDenominator,
		SourceEvidenceJSON: evidence, Memo: request.Memo, ChangeReason: request.ChangeReason,
		ReconciliationOverride:    request.ReconciliationOverride,
		GainImpactAcknowledgement: request.GainImpactAcknowledgement,
	}
}

func toShareExchangePlanResponse(plan app.ShareExchangePlan) shareExchangePlanResponse {
	out := shareExchangePlanResponse{RatioNumerator: plan.RatioNumerator, RatioDenominator: plan.RatioDenominator,
		SourceQuantityValue: plan.SourceQuantityValue, SourceQuantityScale: plan.SourceQuantityScale,
		DestinationQuantityValue: plan.DestinationQuantityValue, DestinationQuantityScale: plan.DestinationQuantityScale,
		Links: make([]shareExchangeLinkResponse, 0, len(plan.Links))}
	for _, link := range plan.Links {
		response := shareExchangeLinkResponse{SourceLotID: link.SourceLotID, CostCommodityID: link.CostCommodityID,
			SourceQuantityValue: link.SourceQuantityValue, SourceQuantityScale: link.SourceQuantityScale,
			DestinationQuantityValue: link.DestinationQuantityValue, DestinationQuantityScale: link.DestinationQuantityScale,
			BasisKnowledge: link.BasisKnowledge, OriginalDateKnowledge: link.OriginalDateKnowledge}
		if link.DestinationLotID > 0 {
			id := link.DestinationLotID
			response.DestinationLotID = &id
		}
		if link.BasisKnowledge != "unknown" {
			value, scale := exact.New(link.CarriedBasisValue).String(), link.CarriedBasisScale
			response.CarriedBasisValue, response.CarriedBasisScale = &value, &scale
		}
		if link.OriginalAcquiredOn != "" {
			date := link.OriginalAcquiredOn
			response.OriginalAcquiredOn = &date
		}
		out.Links = append(out.Links, response)
	}
	return out
}

func shareExchange(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request shareExchangeRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		result, err := investmentService.ShareExchange(r.Context(), shareExchangeInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "share exchange", err)
			return
		}
		writeJSON(w, http.StatusCreated, shareExchangeResponse{
			Transaction: toTransactionResponse(result.Transaction), Plan: toShareExchangePlanResponse(result.Plan),
		})
	}))
}

func shareExchangePreview(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		var request shareExchangeRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		preview, err := investmentService.PreviewShareExchange(r.Context(), shareExchangeInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "share exchange preview", err)
			return
		}
		impact := toReconciliationImpactResponse(preview.Impact)
		gainImpact, err := toGainImpactResponse(preview.Impact.GainImpact)
		if err != nil {
			writeAPIError(w, http.StatusUnprocessableEntity, "LEDGER_OVERFLOW", "gain impact value exceeds the coefficient range")
			return
		}
		impact.GainImpact = gainImpact
		writeJSON(w, http.StatusOK, shareExchangePreviewResponse{
			Plan: toShareExchangePlanResponse(preview.Plan), Impact: impact,
		})
	}
}
