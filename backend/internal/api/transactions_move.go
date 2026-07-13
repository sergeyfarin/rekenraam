package api

import (
	"log/slog"
	"net/http"
	"strconv"

	"rekenraam/backend/internal/app"
)

type moveRequest struct {
	Direction              string `json:"direction"`
	ReconciliationOverride bool   `json:"reconciliation_override"`
}

func moveTransaction(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, ok := readTransactionID(w, r)
		if !ok {
			return
		}
		var req moveRequest
		if err := decodeJSONBody(r, &req); err != nil {
			writeDecodeError(w, err)
			return
		}
		transaction, err := transactionService.MoveTransaction(r.Context(), app.MoveTransactionInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			TransactionID: transactionID,
			Direction:     req.Direction,
		})
		if err != nil {
			writeTransactionServiceError(w, r, logger, "move transaction", err)
			return
		}
		writeJSON(w, http.StatusOK, toTransactionResponse(transaction))
	}))
}

func movePosting(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		accountID, ok := readAccountID(w, r)
		if !ok {
			return
		}
		postingLineID, ok := readPostingLineIDParam(w, r)
		if !ok {
			return
		}
		var req moveRequest
		if err := decodeJSONBody(r, &req); err != nil {
			writeDecodeError(w, err)
			return
		}
		transaction, err := transactionService.MovePosting(r.Context(), app.MovePostingInput{
			OwnerUserID:            owner.ID,
			AuthSessionID:          authenticatedSessionID(r),
			RequestID:              RequestIDFromContext(r.Context()),
			OriginType:             "browser_api",
			AccountID:              accountID,
			PostingLineID:          postingLineID,
			Direction:              req.Direction,
			ReconciliationOverride: req.ReconciliationOverride,
		})
		if err != nil {
			writeTransactionServiceError(w, r, logger, "move posting", err)
			return
		}
		writeJSON(w, http.StatusOK, toTransactionResponse(transaction))
	}))
}

func readPostingLineIDParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := r.PathValue("posting_line_id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "posting line id is invalid")
		return 0, false
	}
	return id, true
}
