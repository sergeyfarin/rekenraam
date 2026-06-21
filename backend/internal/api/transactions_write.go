package api

import (
	"context"
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
)

func createTransaction(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		var request transactionRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}

		transaction, err := transactionService.CreateTransaction(r.Context(), app.CreateTransactionInput{
			OwnerUserID:            owner.ID,
			AuthSessionID:          authenticatedSessionID(r),
			RequestID:              RequestIDFromContext(r.Context()),
			OriginType:             "browser_api",
			Operation:              "transaction.create",
			Spec:                   toTransactionInput(request),
			ChangeReason:           request.ChangeReason,
			ReconciliationOverride: request.ReconciliationOverride,
		})
		if err != nil {
			writeTransactionServiceError(w, r, logger, "create transaction", err)
			return
		}

		writeJSON(w, http.StatusCreated, toTransactionResponse(transaction))
	}))
}

func updateTransaction(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, ok := readTransactionID(w, r)
		if !ok {
			return
		}

		var request transactionRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}

		transaction, err := transactionService.UpdateTransaction(r.Context(), app.UpdateTransactionInput{
			OwnerUserID:            owner.ID,
			AuthSessionID:          authenticatedSessionID(r),
			RequestID:              RequestIDFromContext(r.Context()),
			OriginType:             "browser_api",
			Operation:              "transaction.update",
			TransactionID:          transactionID,
			Spec:                   toTransactionInput(request),
			ChangeReason:           request.ChangeReason,
			ReconciliationOverride: request.ReconciliationOverride,
		})
		if err != nil {
			writeTransactionServiceError(w, r, logger, "update transaction", err)
			return
		}

		writeJSON(w, http.StatusOK, toTransactionResponse(transaction))
	}))
}

func postTransaction(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, ok := readTransactionID(w, r)
		if !ok {
			return
		}

		var request voidTransactionRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}

		transaction, err := transactionService.PostTransaction(r.Context(), app.PostTransactionInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			TransactionID: transactionID,
			ChangeReason:  request.ChangeReason,
		})
		if err != nil {
			writeTransactionServiceError(w, r, logger, "post transaction", err)
			return
		}

		writeJSON(w, http.StatusOK, toTransactionResponse(transaction))
	}))
}

func voidTransaction(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, ok := readTransactionID(w, r)
		if !ok {
			return
		}

		var request voidTransactionRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}

		transaction, err := transactionService.VoidTransaction(r.Context(), app.VoidTransactionInput{
			OwnerUserID:            owner.ID,
			AuthSessionID:          authenticatedSessionID(r),
			RequestID:              RequestIDFromContext(r.Context()),
			OriginType:             "browser_api",
			TransactionID:          transactionID,
			ChangeReason:           request.ChangeReason,
			ReconciliationOverride: request.ReconciliationOverride,
		})
		if err != nil {
			writeTransactionServiceError(w, r, logger, "void transaction", err)
			return
		}

		writeJSON(w, http.StatusOK, toTransactionResponse(transaction))
	}))
}

func unvoidTransaction(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return transactionLifecycleHandler(logger, authService, transactionService, options, "unvoid transaction", transactionService.UnvoidTransaction)
}

func softDeleteTransaction(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return transactionLifecycleHandler(logger, authService, transactionService, options, "soft-delete transaction", transactionService.SoftDeleteTransaction)
}

func restoreTransaction(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return transactionLifecycleHandler(logger, authService, transactionService, options, "restore transaction", transactionService.RestoreTransaction)
}

func transactionLifecycleHandler(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions, action string, mutate func(context.Context, app.TransactionLifecycleInput) (app.Transaction, error)) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, ok := readTransactionID(w, r)
		if !ok {
			return
		}
		var request voidTransactionRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}
		transaction, err := mutate(r.Context(), app.TransactionLifecycleInput{
			OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r),
			RequestID: RequestIDFromContext(r.Context()), OriginType: "browser_api",
			TransactionID: transactionID, ChangeReason: request.ChangeReason,
			ReconciliationOverride: request.ReconciliationOverride,
		})
		if err != nil {
			writeTransactionServiceError(w, r, logger, action, err)
			return
		}
		writeJSON(w, http.StatusOK, toTransactionResponse(transaction))
	}))
}

func correctTransaction(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		correctedTransactionID, ok := readTransactionID(w, r)
		if !ok {
			return
		}

		var request transactionRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}

		transaction, err := transactionService.CreateTransaction(r.Context(), app.CreateTransactionInput{
			OwnerUserID:               owner.ID,
			AuthSessionID:             authenticatedSessionID(r),
			RequestID:                 RequestIDFromContext(r.Context()),
			OriginType:                "browser_api",
			Operation:                 "transaction.correct",
			CorrectionOfTransactionID: &correctedTransactionID,
			Spec:                      toTransactionInput(request),
			ChangeReason:              request.ChangeReason,
		})
		if err != nil {
			writeTransactionServiceError(w, r, logger, "correct transaction", err)
			return
		}

		writeJSON(w, http.StatusCreated, toTransactionResponse(transaction))
	}))
}

func deleteDraftTransaction(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedMutationOwner(w, r); !ok {
			return
		}
		transactionID, ok := readTransactionID(w, r)
		if !ok {
			return
		}

		if err := transactionService.DeleteDraftTransaction(r.Context(), app.DeleteDraftTransactionInput{TransactionID: transactionID}); err != nil {
			writeTransactionServiceError(w, r, logger, "delete draft transaction", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}))
}

func approveTransaction(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, ok := readTransactionID(w, r)
		if !ok {
			return
		}

		var request voidTransactionRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}

		transaction, err := transactionService.ApproveTransaction(r.Context(), app.ApproveTransactionInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			TransactionID: transactionID,
			ChangeReason:  request.ChangeReason,
		})
		if err != nil {
			writeTransactionServiceError(w, r, logger, "approve transaction", err)
			return
		}

		writeJSON(w, http.StatusOK, toTransactionResponse(transaction))
	}))
}
