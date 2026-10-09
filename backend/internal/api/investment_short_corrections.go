package api

import (
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
)

// Native short-sale and cover correction (#175). The request and response
// shapes are the buy/sale correction ones: a reversal takes a reason, a
// replacement a reason and the corrected trade.

func shortCorrectionTransactionID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	return readPathInt64(w, r, "transaction_id", "transaction id")
}

func reverseShortOperation(logger *slog.Logger, authService *app.AuthService, options HandlerOptions, action string,
	reverse func(*http.Request, app.Owner, int64, investmentSaleReversalRequest) (app.Transaction, error),
) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, ok := shortCorrectionTransactionID(w, r)
		if !ok {
			return
		}
		var request investmentSaleReversalRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		transaction, err := reverse(r, owner, transactionID, request)
		if err != nil {
			writeInvestmentServiceError(w, r, logger, action, err)
			return
		}
		writeJSON(w, http.StatusCreated, investmentSaleReversalResponse{Transaction: toTransactionResponse(transaction), CorrectedTransactionID: transactionID})
	}))
}

func shortCorrectionImpact[Request any](logger *slog.Logger, authService *app.AuthService, action string,
	preview func(*http.Request, app.Owner, int64, Request) (app.ReconciliationImpact, error),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		transactionID, ok := shortCorrectionTransactionID(w, r)
		if !ok {
			return
		}
		var request Request
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		impact, err := preview(r, owner, transactionID, request)
		if err != nil {
			writeInvestmentServiceError(w, r, logger, action, err)
			return
		}
		writeReconciliationImpact(w, impact)
	}
}

func reverseShortSaleHandler(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return reverseShortOperation(logger, authService, options, "reverse short sale",
		func(r *http.Request, owner app.Owner, transactionID int64, request investmentSaleReversalRequest) (app.Transaction, error) {
			return investmentService.ReverseShortSale(r.Context(), app.ReverseInvestmentBuyInput{
				OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
				TransactionID: transactionID, Reason: request.Reason,
				ReconciliationOverride: request.ReconciliationOverride, GainImpactAcknowledgement: request.GainImpactAcknowledgement,
			})
		})
}

func reverseShortSaleImpactHandler(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return shortCorrectionImpact(logger, authService, "preview short sale reversal",
		func(r *http.Request, owner app.Owner, transactionID int64, request investmentSaleReversalRequest) (app.ReconciliationImpact, error) {
			return investmentService.ReverseShortSaleReconciliationImpact(r.Context(), app.ReverseInvestmentBuyInput{
				OwnerUserID: owner.ID, TransactionID: transactionID, Reason: request.Reason,
			})
		})
}

func reverseShortCoverHandler(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return reverseShortOperation(logger, authService, options, "reverse short cover",
		func(r *http.Request, owner app.Owner, transactionID int64, request investmentSaleReversalRequest) (app.Transaction, error) {
			return investmentService.ReverseShortCover(r.Context(), app.ReverseInvestmentSaleInput{
				OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
				OriginType: "browser_api", TransactionID: transactionID, Reason: request.Reason,
				ReconciliationOverride: request.ReconciliationOverride, GainImpactAcknowledgement: request.GainImpactAcknowledgement,
			})
		})
}

func reverseShortCoverImpactHandler(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return shortCorrectionImpact(logger, authService, "preview short cover reversal",
		func(r *http.Request, owner app.Owner, transactionID int64, request investmentSaleReversalRequest) (app.ReconciliationImpact, error) {
			return investmentService.ReverseShortCoverReconciliationImpact(r.Context(), app.ReverseInvestmentSaleInput{
				OwnerUserID: owner.ID, TransactionID: transactionID, Reason: request.Reason,
			})
		})
}

func replaceShortSaleHandler(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, ok := shortCorrectionTransactionID(w, r)
		if !ok {
			return
		}
		var request investmentBuyReplacementRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		result, err := investmentService.ReplaceShortSale(r.Context(), app.ReplaceInvestmentBuyInput{
			OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r),
			RequestID: RequestIDFromContext(r.Context()), TransactionID: transactionID,
			Reason: request.Reason, ReconciliationOverride: request.ReconciliationOverride,
			Replacement:               toInvestmentTradeInput(owner, r, request.Replacement),
			GainImpactAcknowledgement: request.GainImpactAcknowledgement,
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "replace short sale", err)
			return
		}
		writeJSON(w, http.StatusCreated, investmentBuyReplacementResponse{
			InverseTransaction: toTransactionResponse(result.Inverse), Replacement: toInvestmentTradeResponse(result.Replacement),
			CorrectedTransactionID: transactionID,
		})
	}))
}

func replaceShortSaleImpactHandler(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return shortCorrectionImpact(logger, authService, "preview short sale replacement",
		func(r *http.Request, owner app.Owner, transactionID int64, request investmentBuyReplacementRequest) (app.ReconciliationImpact, error) {
			return investmentService.ReplaceShortSaleReconciliationImpact(r.Context(), app.ReplaceInvestmentBuyInput{
				OwnerUserID: owner.ID, TransactionID: transactionID, Reason: request.Reason,
				Replacement: toInvestmentTradeInput(owner, r, request.Replacement),
			})
		})
}

func replaceShortCoverHandler(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, ok := shortCorrectionTransactionID(w, r)
		if !ok {
			return
		}
		var request investmentSaleReplacementRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		result, err := investmentService.ReplaceShortCover(r.Context(), app.ReplaceInvestmentSaleInput{
			OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r),
			RequestID: RequestIDFromContext(r.Context()), TransactionID: transactionID,
			Reason: request.Reason, ReconciliationOverride: request.ReconciliationOverride,
			Replacement:               toInvestmentTradeInput(owner, r, request.Replacement),
			GainImpactAcknowledgement: request.GainImpactAcknowledgement,
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "replace short cover", err)
			return
		}
		writeJSON(w, http.StatusCreated, investmentSaleReplacementResponse{
			InverseTransaction: toTransactionResponse(result.Inverse), Replacement: toInvestmentTradeResponse(result.Replacement),
			CorrectedTransactionID: transactionID,
		})
	}))
}

func replaceShortCoverImpactHandler(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return shortCorrectionImpact(logger, authService, "preview short cover replacement",
		func(r *http.Request, owner app.Owner, transactionID int64, request investmentSaleReplacementRequest) (app.ReconciliationImpact, error) {
			return investmentService.ReplaceShortCoverReconciliationImpact(r.Context(), app.ReplaceInvestmentSaleInput{
				OwnerUserID: owner.ID, TransactionID: transactionID, Reason: request.Reason,
				Replacement: toInvestmentTradeInput(owner, r, request.Replacement),
			})
		})
}
