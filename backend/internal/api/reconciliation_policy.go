package api

import (
	"errors"
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
	State             string                            `json:"state"`
	LastStatementDate *string                           `json:"last_statement_date"`
	Commodities       []commodityReconciliationResponse `json:"commodities"`
}

type commodityReconciliationResponse struct {
	CommodityID       int64   `json:"commodity_id"`
	CommodityCode     string  `json:"commodity_code"`
	State             string  `json:"state"`
	LastStatementDate *string `json:"last_statement_date"`
}

type foldedAccountReconciliationResponse struct {
	AccountID int64 `json:"account_id"`
	accountReconciliationResponse
}

func toReconciliationPolicyResponse(policy app.ReconciliationPolicy) reconciliationPolicyResponse {
	return reconciliationPolicyResponse{OutOfDateAfterDays: policy.OutOfDateAfterDays, UpdatedAt: optionalHomeString(policy.UpdatedAt)}
}

func toAccountReconciliationResponse(age *app.AccountReconciliationAge) *accountReconciliationResponse {
	if age == nil {
		return nil
	}
	response := &accountReconciliationResponse{State: age.State, LastStatementDate: optionalHomeString(age.LastStatementDate), Commodities: toCommodityReconciliationResponses(age.Commodities)}
	return response
}

func toCommodityReconciliationResponses(ages []app.CommodityReconciliationAge) []commodityReconciliationResponse {
	response := make([]commodityReconciliationResponse, 0, len(ages))
	for _, age := range ages {
		response = append(response, commodityReconciliationResponse{CommodityID: age.CommodityID, CommodityCode: age.CommodityCode, State: age.State, LastStatementDate: optionalHomeString(age.LastStatementDate)})
	}
	return response
}

func toFoldedAccountReconciliationResponses(ages []app.AccountReconciliationAge) []foldedAccountReconciliationResponse {
	response := make([]foldedAccountReconciliationResponse, 0, len(ages))
	for _, age := range ages {
		response = append(response, foldedAccountReconciliationResponse{AccountID: age.AccountID, accountReconciliationResponse: *toAccountReconciliationResponse(&age)})
	}
	return response
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

type accountReconciliationLookupResponse struct {
	AccountID          int64                          `json:"account_id"`
	OutOfDateAfterDays int                            `json:"out_of_date_after_days"`
	Reconciliation     *accountReconciliationResponse `json:"reconciliation"`
}

// readAccountReconciliation is one account's judgement, read through that
// account alone, for the Reconcile form's commodity choice (#594 review).
func readAccountReconciliation(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		accountID, ok := readAccountID(w, r)
		if !ok {
			return
		}
		age, window, err := transactionService.AccountReconciliationAge(r.Context(), owner.ID, accountID)
		if err != nil {
			if errors.Is(err, app.ErrAccountNotFound) {
				writeAccountServiceError(w, r, logger, "read account reconciliation", err)
			} else {
				writeTransactionServiceError(w, r, logger, "read account reconciliation", err)
			}
			return
		}
		writeJSON(w, http.StatusOK, accountReconciliationLookupResponse{AccountID: accountID, OutOfDateAfterDays: window, Reconciliation: toAccountReconciliationResponse(age)})
	}
}
