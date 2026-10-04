package api

import (
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
)

// T-115: native corrections for cash dividends and reinvested dividends.
// Reversals reuse the trade-reversal request and response shapes; previews
// run each command's writer in a rolled-back transaction.

type investmentDividendReplacementRequest struct {
	Reason                 string          `json:"reason"`
	ReconciliationOverride bool            `json:"reconciliation_override"`
	Replacement            dividendRequest `json:"replacement"`
}

type investmentDividendReplacementResponse struct {
	InverseTransaction     transactionResponse `json:"inverse_transaction"`
	ReplacementTransaction transactionResponse `json:"replacement_transaction"`
	CorrectedTransactionID int64               `json:"corrected_transaction_id"`
}

type investmentReinvestmentReplacementRequest struct {
	Reason                    string                    `json:"reason"`
	ReconciliationOverride    bool                      `json:"reconciliation_override"`
	Replacement               reinvestedDividendRequest `json:"replacement"`
	GainImpactAcknowledgement string                    `json:"gain_impact_acknowledgement,omitempty"`
}

// correctionRoute decodes the path transaction and JSON body shared by every
// correction command and preview.
func correctionRoute[T any](w http.ResponseWriter, r *http.Request) (int64, T, bool) {
	var request T
	transactionID, ok := readPathInt64(w, r, "transaction_id", "transaction id")
	if !ok {
		return 0, request, false
	}
	if err := decodeJSONBody(r, &request); err != nil {
		writeDecodeError(w, err)
		return 0, request, false
	}
	return transactionID, request, true
}

func reverseInvestmentDividend(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, request, ok := correctionRoute[investmentSaleReversalRequest](w, r)
		if !ok {
			return
		}
		transaction, err := investmentService.ReverseDividend(r.Context(), app.ReverseInvestmentDividendInput{
			OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
			TransactionID: transactionID, Reason: request.Reason, ReconciliationOverride: request.ReconciliationOverride,
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "reverse investment dividend", err)
			return
		}
		writeJSON(w, http.StatusCreated, investmentSaleReversalResponse{Transaction: toTransactionResponse(transaction), CorrectedTransactionID: transactionID})
	}))
}

func reverseInvestmentDividendReconciliationImpact(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		transactionID, request, ok := correctionRoute[investmentSaleReversalRequest](w, r)
		if !ok {
			return
		}
		impact, err := investmentService.ReverseDividendReconciliationImpact(r.Context(), app.ReverseInvestmentDividendInput{
			OwnerUserID: owner.ID, TransactionID: transactionID, Reason: request.Reason,
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "preview dividend reversal reconciliation impact", err)
			return
		}
		writeReconciliationImpact(w, impact)
	}
}

func replaceInvestmentDividend(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, request, ok := correctionRoute[investmentDividendReplacementRequest](w, r)
		if !ok {
			return
		}
		result, err := investmentService.ReplaceDividend(r.Context(), app.ReplaceInvestmentDividendInput{
			OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
			TransactionID: transactionID, Reason: request.Reason, ReconciliationOverride: request.ReconciliationOverride,
			Replacement: toDividendInput(owner, r, request.Replacement),
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "replace investment dividend", err)
			return
		}
		writeJSON(w, http.StatusCreated, investmentDividendReplacementResponse{
			InverseTransaction:     toTransactionResponse(result.Inverse),
			ReplacementTransaction: toTransactionResponse(result.Replacement),
			CorrectedTransactionID: transactionID,
		})
	}))
}

func replaceInvestmentDividendReconciliationImpact(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		transactionID, request, ok := correctionRoute[investmentDividendReplacementRequest](w, r)
		if !ok {
			return
		}
		impact, err := investmentService.ReplaceDividendReconciliationImpact(r.Context(), app.ReplaceInvestmentDividendInput{
			OwnerUserID: owner.ID, TransactionID: transactionID, Reason: request.Reason,
			Replacement: toDividendInput(owner, r, request.Replacement),
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "preview dividend replacement reconciliation impact", err)
			return
		}
		writeReconciliationImpact(w, impact)
	}
}

func reverseInvestmentReinvestedDividend(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, request, ok := correctionRoute[investmentSaleReversalRequest](w, r)
		if !ok {
			return
		}
		transaction, err := investmentService.ReverseReinvestedDividend(r.Context(), app.ReverseReinvestedDividendInput{
			OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
			TransactionID: transactionID, Reason: request.Reason, ReconciliationOverride: request.ReconciliationOverride,
			GainImpactAcknowledgement: request.GainImpactAcknowledgement,
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "reverse reinvested dividend", err)
			return
		}
		writeJSON(w, http.StatusCreated, investmentSaleReversalResponse{Transaction: toTransactionResponse(transaction), CorrectedTransactionID: transactionID})
	}))
}

func reverseInvestmentReinvestedDividendReconciliationImpact(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		transactionID, request, ok := correctionRoute[investmentSaleReversalRequest](w, r)
		if !ok {
			return
		}
		impact, err := investmentService.ReverseReinvestedDividendReconciliationImpact(r.Context(), app.ReverseReinvestedDividendInput{
			OwnerUserID: owner.ID, TransactionID: transactionID, Reason: request.Reason,
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "preview reinvested dividend reversal reconciliation impact", err)
			return
		}
		writeReconciliationImpact(w, impact)
	}
}

func replaceInvestmentReinvestedDividend(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, request, ok := correctionRoute[investmentReinvestmentReplacementRequest](w, r)
		if !ok {
			return
		}
		result, err := investmentService.ReplaceReinvestedDividend(r.Context(), app.ReplaceReinvestedDividendInput{
			OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
			TransactionID: transactionID, Reason: request.Reason, ReconciliationOverride: request.ReconciliationOverride,
			Replacement:               toReinvestedDividendInput(owner, r, request.Replacement),
			GainImpactAcknowledgement: request.GainImpactAcknowledgement,
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "replace reinvested dividend", err)
			return
		}
		writeJSON(w, http.StatusCreated, investmentBuyReplacementResponse{
			InverseTransaction:     toTransactionResponse(result.Inverse),
			Replacement:            toInvestmentTradeResponse(result.Replacement),
			CorrectedTransactionID: transactionID,
		})
	}))
}

func replaceInvestmentReinvestedDividendReconciliationImpact(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		transactionID, request, ok := correctionRoute[investmentReinvestmentReplacementRequest](w, r)
		if !ok {
			return
		}
		impact, err := investmentService.ReplaceReinvestedDividendReconciliationImpact(r.Context(), app.ReplaceReinvestedDividendInput{
			OwnerUserID: owner.ID, TransactionID: transactionID, Reason: request.Reason,
			Replacement: toReinvestedDividendInput(owner, r, request.Replacement),
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "preview reinvested dividend replacement reconciliation impact", err)
			return
		}
		writeReconciliationImpact(w, impact)
	}
}
