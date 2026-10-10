package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/exact"
)

// Spin-off (#180): every parent lot keeps its units and moves an exact
// fraction of its basis to a new lot of the distributed instrument.
type spinOffRequest struct {
	EffectiveOn            string            `json:"effective_on"`
	HoldingAccountID       int64             `json:"holding_account_id"`
	DestinationHoldingID   int64             `json:"destination_holding_account_id,omitempty"`
	CommodityID            int64             `json:"commodity_id"`
	DestinationCommodityID int64             `json:"destination_commodity_id"`
	RatioNumerator         int64             `json:"ratio_numerator"`
	RatioDenominator       int64             `json:"ratio_denominator"`
	BasisFractionValue     exact.Coefficient `json:"basis_fraction_value"`
	BasisFractionScale     int               `json:"basis_fraction_scale"`
	SourceEvidence         json.RawMessage   `json:"source_evidence,omitempty"`
	Memo                   string            `json:"memo"`
	ChangeReason           string            `json:"change_reason"`
	ReconciliationOverride bool              `json:"reconciliation_override"`
	// GainImpactAcknowledgement acknowledges the gains a backdated spin-off
	// revises (#183).
	GainImpactAcknowledgement string `json:"gain_impact_acknowledgement,omitempty"`
}

type spinOffLinkResponse struct {
	SourceLotID              int64             `json:"source_lot_id"`
	DestinationLotID         *int64            `json:"destination_lot_id"`
	CostCommodityID          int64             `json:"cost_commodity_id"`
	SourceQuantityValue      exact.Coefficient `json:"source_quantity_value"`
	SourceQuantityScale      int               `json:"source_quantity_scale"`
	DestinationQuantityValue exact.Coefficient `json:"destination_quantity_value"`
	DestinationQuantityScale int               `json:"destination_quantity_scale"`
	BasisKnowledge           string            `json:"basis_knowledge"`
	AllocatedBasisValue      *string           `json:"allocated_basis_value"`
	AllocatedBasisScale      *int              `json:"allocated_basis_scale"`
	RemainingBasisValue      *string           `json:"remaining_basis_value"`
	RemainingBasisScale      *int              `json:"remaining_basis_scale"`
	OriginalDateKnowledge    string            `json:"original_date_knowledge"`
	OriginalAcquiredOn       *string           `json:"original_acquired_on"`
}

// spinOffBasisTotalResponse is what a spin-off divides in one cost currency;
// the basis is null when any lot in it is unknown, and the remaining basis is
// null outside a preview or result.
type spinOffBasisTotalResponse struct {
	CostCommodityID          int64             `json:"cost_commodity_id"`
	SourceQuantityValue      exact.Coefficient `json:"source_quantity_value"`
	SourceQuantityScale      int               `json:"source_quantity_scale"`
	DestinationQuantityValue exact.Coefficient `json:"destination_quantity_value"`
	DestinationQuantityScale int               `json:"destination_quantity_scale"`
	BasisKnowledge           string            `json:"basis_knowledge"`
	AllocatedBasisValue      *string           `json:"allocated_basis_value"`
	AllocatedBasisScale      *int              `json:"allocated_basis_scale"`
	RemainingBasisValue      *string           `json:"remaining_basis_value"`
	RemainingBasisScale      *int              `json:"remaining_basis_scale"`
	UnknownLots              int               `json:"unknown_lots"`
}

type spinOffPlanResponse struct {
	RatioNumerator           int64                       `json:"ratio_numerator"`
	RatioDenominator         int64                       `json:"ratio_denominator"`
	BasisFractionValue       exact.Coefficient           `json:"basis_fraction_value"`
	BasisFractionScale       int                         `json:"basis_fraction_scale"`
	Links                    []spinOffLinkResponse       `json:"links"`
	DestinationQuantityValue exact.Coefficient           `json:"destination_quantity_value"`
	DestinationQuantityScale int                         `json:"destination_quantity_scale"`
	BasisTotals              []spinOffBasisTotalResponse `json:"basis_totals"`
}

type spinOffPreviewResponse struct {
	Plan   spinOffPlanResponse          `json:"plan"`
	Impact reconciliationImpactResponse `json:"impact"`
}

type spinOffResponse struct {
	Transaction transactionResponse `json:"transaction"`
	Plan        spinOffPlanResponse `json:"plan"`
}

// investmentCorrectionSpinOffTerms explain an effective spin-off in the
// correction chain (#180). The parent's remaining basis is null there.
type investmentCorrectionSpinOffTerms struct {
	EffectiveOn                 string              `json:"effective_on"`
	HoldingAccountID            int64               `json:"holding_account_id"`
	DestinationHoldingAccountID int64               `json:"destination_holding_account_id"`
	CommodityID                 int64               `json:"commodity_id"`
	DestinationCommodityID      int64               `json:"destination_commodity_id"`
	SourceEvidence              json.RawMessage     `json:"source_evidence"`
	Plan                        spinOffPlanResponse `json:"plan"`
}

func toInvestmentCorrectionSpinOffTerms(terms *app.InvestmentCorrectionSpinOffTerms) *investmentCorrectionSpinOffTerms {
	if terms == nil {
		return nil
	}
	evidence := terms.SourceEvidenceJSON
	if evidence == "" {
		evidence = "{}"
	}
	return &investmentCorrectionSpinOffTerms{
		EffectiveOn: terms.EffectiveOn, HoldingAccountID: terms.HoldingAccountID,
		DestinationHoldingAccountID: terms.DestinationHoldingAccountID, CommodityID: terms.CommodityID,
		DestinationCommodityID: terms.DestinationCommodityID, SourceEvidence: json.RawMessage(evidence),
		Plan: toSpinOffPlanResponse(terms.Plan, false),
	}
}

func spinOffInput(owner app.Owner, r *http.Request, request spinOffRequest) app.SpinOffInput {
	evidence := rawJSONText(request.SourceEvidence)
	if evidence == "null" {
		evidence = ""
	}
	return app.SpinOffInput{
		OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r),
		RequestID: RequestIDFromContext(r.Context()), EffectiveOn: request.EffectiveOn,
		HoldingAccountID: request.HoldingAccountID, DestinationHoldingAccountID: request.DestinationHoldingID,
		CommodityID: request.CommodityID, DestinationCommodityID: request.DestinationCommodityID,
		RatioNumerator: request.RatioNumerator, RatioDenominator: request.RatioDenominator,
		BasisFractionValue: request.BasisFractionValue, BasisFractionScale: request.BasisFractionScale,
		SourceEvidenceJSON: evidence, Memo: request.Memo, ChangeReason: request.ChangeReason,
		ReconciliationOverride:    request.ReconciliationOverride,
		GainImpactAcknowledgement: request.GainImpactAcknowledgement,
	}
}

// optionalAmount is an exact amount on the wire, or null.
func optionalAmount(value exact.Coefficient, scale int, present bool) (*string, *int) {
	if !present {
		return nil, nil
	}
	text := value.String()
	return &text, &scale
}

func toSpinOffPlanResponse(plan app.SpinOffPlan, withRemaining bool) spinOffPlanResponse {
	out := spinOffPlanResponse{RatioNumerator: plan.RatioNumerator, RatioDenominator: plan.RatioDenominator,
		BasisFractionValue: plan.BasisFractionValue, BasisFractionScale: plan.BasisFractionScale,
		DestinationQuantityValue: plan.DestinationQuantityValue, DestinationQuantityScale: plan.DestinationQuantityScale,
		Links: make([]spinOffLinkResponse, 0, len(plan.Links))}
	for _, link := range plan.Links {
		known := link.BasisKnowledge != "unknown"
		response := spinOffLinkResponse{SourceLotID: link.SourceLotID, CostCommodityID: link.CostCommodityID,
			SourceQuantityValue: link.SourceQuantityValue, SourceQuantityScale: link.SourceQuantityScale,
			DestinationQuantityValue: link.DestinationQuantityValue, DestinationQuantityScale: link.DestinationQuantityScale,
			BasisKnowledge: link.BasisKnowledge, OriginalDateKnowledge: link.OriginalDateKnowledge}
		if link.DestinationLotID > 0 {
			id := link.DestinationLotID
			response.DestinationLotID = &id
		}
		response.AllocatedBasisValue, response.AllocatedBasisScale = optionalAmount(
			exact.New(link.AllocatedBasisValue), link.AllocatedBasisScale, known)
		response.RemainingBasisValue, response.RemainingBasisScale = optionalAmount(
			exact.New(link.RemainingBasisValue), link.RemainingBasisScale, known && withRemaining)
		if link.OriginalAcquiredOn != "" {
			date := link.OriginalAcquiredOn
			response.OriginalAcquiredOn = &date
		}
		out.Links = append(out.Links, response)
	}
	out.BasisTotals = make([]spinOffBasisTotalResponse, 0, len(plan.BasisTotals))
	for _, total := range plan.BasisTotals {
		known := total.BasisKnowledge != "unknown"
		response := spinOffBasisTotalResponse{CostCommodityID: total.CostCommodityID,
			SourceQuantityValue: total.SourceQuantityValue, SourceQuantityScale: total.SourceQuantityScale,
			DestinationQuantityValue: total.DestinationQuantityValue, DestinationQuantityScale: total.DestinationQuantityScale,
			BasisKnowledge: total.BasisKnowledge, UnknownLots: total.UnknownLots}
		response.AllocatedBasisValue, response.AllocatedBasisScale = optionalAmount(
			total.AllocatedBasisValue, total.AllocatedBasisScale, known)
		response.RemainingBasisValue, response.RemainingBasisScale = optionalAmount(
			total.RemainingBasisValue, total.RemainingBasisScale, known && withRemaining)
		out.BasisTotals = append(out.BasisTotals, response)
	}
	return out
}

func spinOff(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request spinOffRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		result, err := investmentService.SpinOff(r.Context(), spinOffInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "spin-off", err)
			return
		}
		writeJSON(w, http.StatusCreated, spinOffResponse{
			Transaction: toTransactionResponse(result.Transaction), Plan: toSpinOffPlanResponse(result.Plan, true),
		})
	}))
}

func spinOffPreview(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		var request spinOffRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		preview, err := investmentService.PreviewSpinOff(r.Context(), spinOffInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "spin-off preview", err)
			return
		}
		writeSpinOffPreview(w, preview)
	}
}

// writeSpinOffPreview writes a plan and its impact, including the gains a
// backdated spin-off or a correction would revise.
func writeSpinOffPreview(w http.ResponseWriter, preview app.SpinOffPreview) {
	impact := toReconciliationImpactResponse(preview.Impact)
	gainImpact, err := toGainImpactResponse(preview.Impact.GainImpact)
	if err != nil {
		writeAPIError(w, http.StatusUnprocessableEntity, "LEDGER_OVERFLOW", "gain impact value exceeds the coefficient range")
		return
	}
	impact.GainImpact = gainImpact
	writeJSON(w, http.StatusOK, spinOffPreviewResponse{Plan: toSpinOffPlanResponse(preview.Plan, true), Impact: impact})
}

// Spin-off correction (#183). Reversal reuses the sale-reversal request and
// response; replacement carries corrected terms for the same parent holding
// and instrument.
type spinOffReplacementRequest struct {
	Reason                    string            `json:"reason"`
	EffectiveOn               string            `json:"effective_on"`
	DestinationHoldingID      int64             `json:"destination_holding_account_id,omitempty"`
	DestinationCommodityID    int64             `json:"destination_commodity_id"`
	RatioNumerator            int64             `json:"ratio_numerator"`
	RatioDenominator          int64             `json:"ratio_denominator"`
	BasisFractionValue        exact.Coefficient `json:"basis_fraction_value"`
	BasisFractionScale        int               `json:"basis_fraction_scale"`
	SourceEvidence            json.RawMessage   `json:"source_evidence,omitempty"`
	Memo                      string            `json:"memo"`
	ReconciliationOverride    bool              `json:"reconciliation_override"`
	GainImpactAcknowledgement string            `json:"gain_impact_acknowledgement,omitempty"`
}

type spinOffReplacementResponse struct {
	Inverse                transactionResponse `json:"inverse"`
	Replacement            transactionResponse `json:"replacement"`
	Plan                   spinOffPlanResponse `json:"plan"`
	CorrectedTransactionID int64               `json:"corrected_transaction_id"`
}

func spinOffReplacementInput(owner app.Owner, r *http.Request, transactionID int64, request spinOffReplacementRequest) app.ReplaceSpinOffInput {
	evidence := rawJSONText(request.SourceEvidence)
	if evidence == "null" {
		evidence = ""
	}
	return app.ReplaceSpinOffInput{
		OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
		TransactionID: transactionID, Reason: request.Reason, EffectiveOn: request.EffectiveOn,
		DestinationHoldingAccountID: request.DestinationHoldingID, DestinationCommodityID: request.DestinationCommodityID,
		RatioNumerator: request.RatioNumerator, RatioDenominator: request.RatioDenominator,
		BasisFractionValue: request.BasisFractionValue, BasisFractionScale: request.BasisFractionScale,
		SourceEvidenceJSON: evidence, Memo: request.Memo,
		ReconciliationOverride:    request.ReconciliationOverride,
		GainImpactAcknowledgement: request.GainImpactAcknowledgement,
	}
}

func reverseSpinOff(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, ok := readPathInt64(w, r, "transaction_id", "transaction id")
		if !ok {
			return
		}
		var request investmentSaleReversalRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		transaction, err := investmentService.ReverseSpinOff(r.Context(), app.ReverseSpinOffInput{
			OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
			TransactionID: transactionID, Reason: request.Reason,
			ReconciliationOverride: request.ReconciliationOverride, GainImpactAcknowledgement: request.GainImpactAcknowledgement,
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "reverse spin-off", err)
			return
		}
		writeJSON(w, http.StatusCreated, investmentSaleReversalResponse{Transaction: toTransactionResponse(transaction), CorrectedTransactionID: transactionID})
	}))
}

func reverseSpinOffReconciliationImpact(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		transactionID, ok := readPathInt64(w, r, "transaction_id", "transaction id")
		if !ok {
			return
		}
		var request investmentSaleReversalRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		impact, err := investmentService.ReverseSpinOffReconciliationImpact(r.Context(), app.ReverseSpinOffInput{
			OwnerUserID: owner.ID, TransactionID: transactionID, Reason: request.Reason,
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "preview spin-off reversal reconciliation impact", err)
			return
		}
		writeReconciliationImpact(w, impact)
	}
}

func replaceSpinOff(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, ok := readPathInt64(w, r, "transaction_id", "transaction id")
		if !ok {
			return
		}
		var request spinOffReplacementRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		result, err := investmentService.ReplaceSpinOff(r.Context(), spinOffReplacementInput(owner, r, transactionID, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "replace spin-off", err)
			return
		}
		writeJSON(w, http.StatusCreated, spinOffReplacementResponse{
			Inverse: toTransactionResponse(result.Inverse), Replacement: toTransactionResponse(result.Replacement),
			Plan: toSpinOffPlanResponse(result.Plan, true), CorrectedTransactionID: result.CorrectedTransactionID,
		})
	}))
}

func replaceSpinOffPreview(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		transactionID, ok := readPathInt64(w, r, "transaction_id", "transaction id")
		if !ok {
			return
		}
		var request spinOffReplacementRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		preview, err := investmentService.PreviewSpinOffReplacement(r.Context(), spinOffReplacementInput(owner, r, transactionID, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "preview spin-off replacement", err)
			return
		}
		writeSpinOffPreview(w, preview)
	}
}
