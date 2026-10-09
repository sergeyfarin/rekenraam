package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
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
