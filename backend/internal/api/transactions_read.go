package api

import (
	"log/slog"
	"net/http"
	"strconv"

	"rekenraam/backend/internal/app"
)

func listTransactions(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		input, ok := readTransactionListInput(w, r)
		if !ok {
			return
		}

		result, err := transactionService.ListTransactions(r.Context(), input)
		if err != nil {
			writeTransactionServiceError(w, r, logger, "list transactions", err)
			return
		}

		writeJSON(w, http.StatusOK, transactionsResponse{
			Transactions: toTransactionResponses(result.Transactions),
			NextCursor:   nullableCursor(result.NextCursor),
		})
	}
}

func readTransaction(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		transactionID, ok := readTransactionID(w, r)
		if !ok {
			return
		}

		transaction, err := transactionService.Transaction(r.Context(), transactionID)
		if err != nil {
			writeTransactionServiceError(w, r, logger, "read transaction", err)
			return
		}

		writeJSON(w, http.StatusOK, toTransactionResponse(transaction))
	}
}

func accountRegister(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		accountID, ok := readAccountID(w, r)
		if !ok {
			return
		}
		input, ok := readTransactionListInput(w, r)
		if !ok {
			return
		}

		result, err := transactionService.Register(r.Context(), accountID, input)
		if err != nil {
			writeTransactionServiceError(w, r, logger, "read account register", err)
			return
		}

		writeJSON(w, http.StatusOK, accountRegisterResponse{
			Entries:    toAccountRegisterEntryResponses(result.Entries),
			NextCursor: nullableCursor(result.NextCursor),
		})
	}
}

func listDeletedTransactions(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		query := r.URL.Query()
		limit := 0
		if query.Get("limit") != "" {
			parsed, err := strconv.Atoi(query.Get("limit"))
			if err != nil || parsed <= 0 {
				writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "limit is invalid")
				return
			}
			limit = parsed
		}

		result, err := transactionService.ListDeletedTransactions(r.Context(), app.ListDeletedTransactionsInput{
			Limit:  limit,
			Cursor: query.Get("cursor"),
		})
		if err != nil {
			writeTransactionServiceError(w, r, logger, "list deleted transactions", err)
			return
		}

		writeJSON(w, http.StatusOK, deletedTransactionsResponse{
			Transactions: toDeletedTransactionResponses(result.Transactions),
			NextCursor:   nullableCursor(result.NextCursor),
		})
	}
}

func toDeletedTransactionResponses(txns []app.DeletedTransaction) []deletedTransactionResponse {
	responses := make([]deletedTransactionResponse, 0, len(txns))
	for _, txn := range txns {
		responses = append(responses, deletedTransactionResponse{
			transactionResponse:            toTransactionResponse(txn.Transaction),
			DeleteReason:                   txn.DeleteReason,
			RestoreBlockedByReconciliation: txn.RestoreBlockedByReconciliation,
		})
	}
	return responses
}

func readTransactionListInput(w http.ResponseWriter, r *http.Request) (app.ListTransactionsInput, bool) {
	query := r.URL.Query()
	accountID, ok := parseOptionalPositiveInt64(w, query.Get("account_id"), "account id")
	if !ok {
		return app.ListTransactionsInput{}, false
	}
	categoryID, ok := parseOptionalPositiveInt64(w, query.Get("category_id"), "category id")
	if !ok {
		return app.ListTransactionsInput{}, false
	}
	payeeID, ok := parseOptionalPositiveInt64(w, query.Get("payee_id"), "payee id")
	if !ok {
		return app.ListTransactionsInput{}, false
	}
	limit := 0
	if query.Get("limit") != "" {
		parsed, err := strconv.Atoi(query.Get("limit"))
		if err != nil || parsed <= 0 {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "limit is invalid")
			return app.ListTransactionsInput{}, false
		}
		limit = parsed
	}

	return app.ListTransactionsInput{
		AccountID:    accountID,
		CategoryID:   categoryID,
		PayeeID:      payeeID,
		Status:       query.Get("status"),
		Kind:         query.Get("kind"),
		NeedsReview:  query.Get("needs_review") == "true",
		Query:        query.Get("q"),
		AfterDate:    query.Get("after_date"),
		BeforeDate:   query.Get("before_date"),
		CategoryType: query.Get("category_type"),
		DateBasis:    query.Get("date_basis"),
		Limit:        limit,
		Cursor:       query.Get("cursor"),
	}, true
}

func nullableCursor(cursor string) *string {
	if cursor == "" {
		return nil
	}
	return &cursor
}

func parseOptionalPositiveInt64(w http.ResponseWriter, value string, field string) (int64, bool) {
	if value == "" {
		return 0, true
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", field+" is invalid")
		return 0, false
	}
	return parsed, true
}

func readTransactionID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	transactionID, err := strconv.ParseInt(r.PathValue("transaction_id"), 10, 64)
	if err != nil || transactionID <= 0 {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "transaction id is invalid")
		return 0, false
	}
	return transactionID, true
}
