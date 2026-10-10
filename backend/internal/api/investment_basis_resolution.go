package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/exact"
)

// investmentBasisResolutionRequest resolves an external transfer in's unknown
// basis with its sourced total in the transfer's cost currency (T-145).
type investmentBasisResolutionRequest struct {
	BasisValue                *moneyCoefficient `json:"basis_value"`
	BasisScale                int               `json:"basis_scale"`
	SourceEvidence            json.RawMessage   `json:"source_evidence,omitempty"`
	Reason                    string            `json:"reason"`
	ReconciliationOverride    bool              `json:"reconciliation_override"`
	GainImpactAcknowledgement string            `json:"gain_impact_acknowledgement,omitempty"`
}

type investmentBasisResolutionResponse struct {
	Transaction           transactionResponse `json:"transaction"`
	ResolvedTransactionID int64               `json:"resolved_transaction_id"`
}

func basisResolutionInput(owner app.Owner, r *http.Request, transactionID int64, request investmentBasisResolutionRequest) (app.ResolveTransferBasisInput, error) {
	if request.BasisValue == nil {
		return app.ResolveTransferBasisInput{}, app.ValidationError{Message: "resolved basis is required"}
	}
	evidence := rawJSONText(request.SourceEvidence)
	if evidence == "null" {
		evidence = ""
	}
	return app.ResolveTransferBasisInput{
		OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
		TransactionID: transactionID, BasisValue: int64(*request.BasisValue), BasisScale: request.BasisScale,
		SourceEvidenceJSON: evidence, Reason: request.Reason,
		ReconciliationOverride:    request.ReconciliationOverride,
		GainImpactAcknowledgement: request.GainImpactAcknowledgement,
	}, nil
}

func resolveInvestmentTransferBasis(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, ok := readPathInt64(w, r, "transaction_id", "transaction id")
		if !ok {
			return
		}
		var request investmentBasisResolutionRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		input, err := basisResolutionInput(owner, r, transactionID, request)
		if err == nil {
			var transaction app.Transaction
			transaction, err = investmentService.ResolveTransferBasis(r.Context(), input)
			if err == nil {
				writeJSON(w, http.StatusCreated, investmentBasisResolutionResponse{
					Transaction: toTransactionResponse(transaction), ResolvedTransactionID: transactionID})
				return
			}
		}
		writeInvestmentServiceError(w, r, logger, "resolve investment transfer basis", err)
	}))
}

func resolveInvestmentTransferBasisReconciliationImpact(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		transactionID, ok := readPathInt64(w, r, "transaction_id", "transaction id")
		if !ok {
			return
		}
		var request investmentBasisResolutionRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		input, err := basisResolutionInput(owner, r, transactionID, request)
		if err == nil {
			var impact app.ReconciliationImpact
			impact, err = investmentService.ResolveTransferBasisImpact(r.Context(), input)
			if err == nil {
				writeReconciliationImpact(w, impact)
				return
			}
		}
		writeInvestmentServiceError(w, r, logger, "preview transfer basis resolution", err)
	}
}

// investmentCorrectionBasisResolutionTerms pre-fill a resolution replacement:
// the sourced basis in the pinned transfer's cost currency (#168).
type investmentCorrectionBasisResolutionTerms struct {
	OperationID     int64             `json:"operation_id"`
	TransactionID   int64             `json:"transaction_id"`
	CostCommodityID int64             `json:"cost_commodity_id"`
	BasisValue      exact.Coefficient `json:"basis_value"`
	BasisScale      int               `json:"basis_scale"`
	SourceEvidence  json.RawMessage   `json:"source_evidence"`
}

func toInvestmentCorrectionBasisResolutionTerms(terms *app.InvestmentCorrectionBasisResolutionTerms) *investmentCorrectionBasisResolutionTerms {
	if terms == nil {
		return nil
	}
	return &investmentCorrectionBasisResolutionTerms{OperationID: terms.OperationID, TransactionID: terms.TransactionID,
		CostCommodityID: terms.CostCommodityID, BasisValue: terms.BasisValue, BasisScale: terms.BasisScale,
		SourceEvidence: json.RawMessage(defaultJSONObject(terms.SourceEvidenceJSON))}
}

type investmentBasisResolutionReplacementResponse struct {
	Inverse                transactionResponse `json:"inverse"`
	Replacement            transactionResponse `json:"replacement"`
	CorrectedTransactionID int64               `json:"corrected_transaction_id"`
	TransferTransactionID  int64               `json:"transfer_transaction_id"`
}

func basisResolutionCorrectionInput(owner app.Owner, r *http.Request, transactionID int64, request investmentBasisResolutionRequest) (app.CorrectTransferBasisResolutionInput, error) {
	resolution, err := basisResolutionInput(owner, r, transactionID, request)
	if err != nil {
		return app.CorrectTransferBasisResolutionInput{}, err
	}
	return app.CorrectTransferBasisResolutionInput{
		OwnerUserID: resolution.OwnerUserID, AuthSessionID: resolution.AuthSessionID, RequestID: resolution.RequestID,
		TransactionID: transactionID, Reason: resolution.Reason, ReconciliationOverride: resolution.ReconciliationOverride,
		GainImpactAcknowledgement: resolution.GainImpactAcknowledgement,
		BasisValue:                resolution.BasisValue, BasisScale: resolution.BasisScale, SourceEvidenceJSON: resolution.SourceEvidenceJSON,
	}, nil
}

func reversalBasisResolutionInput(owner app.Owner, r *http.Request, transactionID int64, request investmentSaleReversalRequest) app.CorrectTransferBasisResolutionInput {
	return app.CorrectTransferBasisResolutionInput{
		OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
		TransactionID: transactionID, Reason: request.Reason, ReconciliationOverride: request.ReconciliationOverride,
		GainImpactAcknowledgement: request.GainImpactAcknowledgement,
	}
}

func replaceInvestmentBasisResolution(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, request, ok := correctionRoute[investmentBasisResolutionRequest](w, r)
		if !ok {
			return
		}
		input, err := basisResolutionCorrectionInput(owner, r, transactionID, request)
		if err == nil {
			var result app.ReplaceTransferBasisResolutionResult
			result, err = investmentService.ReplaceTransferBasisResolution(r.Context(), input)
			if err == nil {
				writeJSON(w, http.StatusCreated, investmentBasisResolutionReplacementResponse{
					Inverse: toTransactionResponse(result.Inverse), Replacement: toTransactionResponse(result.Replacement),
					CorrectedTransactionID: result.CorrectedTransactionID, TransferTransactionID: result.TransferTransactionID})
				return
			}
		}
		writeInvestmentServiceError(w, r, logger, "replace basis resolution", err)
	}))
}

func replaceInvestmentBasisResolutionReconciliationImpact(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		transactionID, request, ok := correctionRoute[investmentBasisResolutionRequest](w, r)
		if !ok {
			return
		}
		input, err := basisResolutionCorrectionInput(owner, r, transactionID, request)
		if err == nil {
			var impact app.ReconciliationImpact
			impact, err = investmentService.ReplaceTransferBasisResolutionImpact(r.Context(), input)
			if err == nil {
				writeReconciliationImpact(w, impact)
				return
			}
		}
		writeInvestmentServiceError(w, r, logger, "preview basis resolution replacement", err)
	}
}

func reverseInvestmentBasisResolution(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, request, ok := correctionRoute[investmentSaleReversalRequest](w, r)
		if !ok {
			return
		}
		transaction, err := investmentService.ReverseTransferBasisResolution(r.Context(), reversalBasisResolutionInput(owner, r, transactionID, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "reverse basis resolution", err)
			return
		}
		writeJSON(w, http.StatusCreated, investmentSaleReversalResponse{Transaction: toTransactionResponse(transaction), CorrectedTransactionID: transactionID})
	}))
}

func reverseInvestmentBasisResolutionReconciliationImpact(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		transactionID, request, ok := correctionRoute[investmentSaleReversalRequest](w, r)
		if !ok {
			return
		}
		impact, err := investmentService.ReverseTransferBasisResolutionImpact(r.Context(), reversalBasisResolutionInput(owner, r, transactionID, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "preview basis resolution reversal", err)
			return
		}
		writeReconciliationImpact(w, impact)
	}
}
