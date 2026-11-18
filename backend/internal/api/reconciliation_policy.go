package api

import (
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
)

// The reconciliation out-of-date window (#482).

type reconciliationPolicyResponse struct {
	OutOfDateAfterDays int     `json:"out_of_date_after_days"`
	UpdatedAt          *string `json:"updated_at"`
}

type reconciliationPolicyRequest struct {
	OutOfDateAfterDays int `json:"out_of_date_after_days"`
}

type accountReconciliationResponse struct {
	State             string  `json:"state"`
	LastStatementDate *string `json:"last_statement_date"`
}

func toReconciliationPolicyResponse(policy app.ReconciliationPolicy) reconciliationPolicyResponse {
	return reconciliationPolicyResponse{OutOfDateAfterDays: policy.OutOfDateAfterDays, UpdatedAt: optionalHomeString(policy.UpdatedAt)}
}

func toAccountReconciliationResponse(age *app.AccountReconciliationAge) *accountReconciliationResponse {
	if age == nil {
		return nil
	}
	return &accountReconciliationResponse{State: age.State, LastStatementDate: optionalHomeString(age.LastStatementDate)}
}

func getReconciliationPolicy(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		policy, err := transactionService.ReconciliationPolicy(r.Context())
		if err != nil {
			writeTransactionServiceError(w, r, logger, "get reconciliation policy", err)
			return
		}
		writeJSON(w, http.StatusOK, toReconciliationPolicyResponse(policy))
	}
}

func saveReconciliationPolicy(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request reconciliationPolicyRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		policy, err := transactionService.SaveReconciliationPolicy(r.Context(), app.SaveReconciliationPolicyInput{
			OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
			OutOfDateAfterDays: request.OutOfDateAfterDays,
		})
		if err != nil {
			writeTransactionServiceError(w, r, logger, "save reconciliation policy", err)
			return
		}
		writeJSON(w, http.StatusOK, toReconciliationPolicyResponse(policy))
	}))
}
