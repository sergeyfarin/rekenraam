package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/exact"
)

type reviewImportDividendRequest struct {
	Resolution                   app.ImportRowResolution `json:"resolution"`
	ExpectedTransactionVersionID int64                   `json:"expected_transaction_version_id"`
	Reason                       string                  `json:"reason"`
	// The review response's marks for accounts the default filled, sent back
	// while the owner keeps them (#664 review).
	IncomeAccountFromDefault      bool `json:"income_account_from_default"`
	WithholdingAccountFromDefault bool `json:"withholding_account_from_default"`
	reconciliationOverrideFields
}

func listImportDividends(logger *slog.Logger, auth *app.AuthService, service *app.ImportService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, auth)
		if !ok {
			return
		}
		input := app.ListImportDividendsInput{OwnerUserID: owner.ID, Cursor: r.URL.Query().Get("cursor")}
		if raw := r.URL.Query().Get("batch_id"); raw != "" {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || value <= 0 {
				writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid batch filter")
				return
			}
			input.BatchID = value
		}
		if raw := r.URL.Query().Get("limit"); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 || value > 100 {
				writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid review limit")
				return
			}
			input.Limit = value
		}
		result, err := service.ListImportDividends(r.Context(), input)
		if err != nil {
			writeImportDividendReviewError(w, r, logger, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func reviewImportDividend(logger *slog.Logger, auth *app.AuthService, service *app.ImportService, options HandlerOptions, preview bool) http.HandlerFunc {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, auth)
		if !ok {
			return
		}
		batchID, ok := readImportBatchID(w, r)
		if !ok {
			return
		}
		rowID, ok := readPathInt64(w, r, "row_id", "row id")
		if !ok {
			return
		}
		var request reviewImportDividendRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		input := app.ReviewImportDividendInput{OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()), BatchID: batchID, RowID: rowID, Resolution: request.Resolution, Reason: request.Reason, ExpectedTransactionVersionID: request.ExpectedTransactionVersionID, ReconciliationOverride: request.checkpointOverride(),
			IncomeAccountFromDefault: request.IncomeAccountFromDefault, WithholdingAccountFromDefault: request.WithholdingAccountFromDefault}
		if preview {
			impact, err := service.ImportDividendReviewImpact(r.Context(), input)
			if err != nil {
				writeImportDividendReviewError(w, r, logger, err)
				return
			}
			writeReconciliationImpact(w, impact)
			return
		}
		result, err := service.ReviewImportDividend(r.Context(), input)
		if err != nil {
			writeImportDividendReviewError(w, r, logger, err)
			return
		}
		writeJSON(w, http.StatusOK, toTransactionResponse(result))
	})
	if preview {
		return handler
	}
	return requireAuthenticatedMutation(logger, auth, options, handler)
}

func writeImportDividendReviewError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, app.ErrImportDividendReviewUnavailable):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "imported dividend review is no longer available")
	case errors.Is(err, app.ErrImportBatchNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "import batch not found")
	case errors.Is(err, app.ErrInvestmentInstrumentNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "investment instrument not found")
	case errors.Is(err, exact.ErrInt64Range):
		writeAPIError(w, http.StatusUnprocessableEntity, "LEDGER_OVERFLOW", "dividend amount exceeds supported precision")
	case errors.Is(err, app.ErrDividendDefaultChanged):
		writeAPIError(w, http.StatusConflict, "DIVIDEND_DEFAULT_CHANGED", err.Error())
	default:
		writeTransactionServiceError(w, r, logger, "review imported dividend", err)
	}
}
