package api

import (
	"errors"
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
)

func writeTransactionServiceError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, action string, err error) {
	var validationError app.ValidationError
	switch {
	case errors.As(err, &validationError):
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", validationError.Error())
	case errors.Is(err, app.ErrTransactionNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "transaction not found")
	case errors.Is(err, app.ErrTransactionProtected):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "transaction requires a corrective workflow")
	case errors.Is(err, app.ErrTransactionPosted):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "posted or voided transaction cannot be deleted")
	case errors.Is(err, app.ErrTransactionVoided):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "voided transaction cannot be edited")
	case errors.Is(err, app.ErrTransactionDeleted):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "soft-deleted transaction must be restored first")
	case errors.Is(err, app.ErrTransactionTag):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "transaction tag is invalid")
	case errors.Is(err, app.ErrReconciliationOverrideRequired):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "reconciliation override is required")
	case errors.Is(err, app.ErrReconciliationNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "reconciliation not found")
	case errors.Is(err, app.ErrReconciliationClosed):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "reconciliation is not open")
	case errors.Is(err, app.ErrReconciliationNotBalanced):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "reconciliation difference must be zero")
	case errors.Is(err, app.ErrReconciliationPosting):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "reconciliation posting is invalid")
	default:
		logger.ErrorContext(r.Context(), action, slog.Any("err", err))
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}
