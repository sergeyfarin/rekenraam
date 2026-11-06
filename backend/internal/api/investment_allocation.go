package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/db"
)

func investmentAllocation(logger *slog.Logger, auth *app.AuthService, investments *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, auth); !ok {
			return
		}
		query := r.URL.Query()
		input := app.ReturnsInput{From: query.Get("from"), To: query.Get("to")}
		var err error
		input.ReportingCommodityID, err = optionalPositiveInt64(query.Get("reporting_currency_id"))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "reporting_currency_id must be positive")
			return
		}
		if raw := query.Get("max_staleness_days"); raw != "" {
			input.MaxStalenessDays, err = strconv.Atoi(raw)
			if err != nil || input.MaxStalenessDays < 1 {
				writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "max_staleness_days must be positive")
				return
			}
		}
		result, err := investments.ReadAllocation(r.Context(), input)
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "read allocation", err)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			app.AllocationRead
			Valuation          *valuationResponse        `json:"valuation"`
			CurrencyAllocation returnsAllocationResponse `json:"currency_allocation"`
		}{result, toValuationResponse(&result.Valuation), toReturnsAllocationResponse(result.CurrencyAllocation)})
	}
}

func saveInvestmentAllocation(logger *slog.Logger, auth *app.AuthService, investments *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, auth, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request struct {
			ExpectedRevision *int64  `json:"expected_revision"`
			AccountIDs       []int64 `json:"account_ids"`
			Targets          []struct {
				CommodityID *int64 `json:"commodity_id"`
				Units       *int   `json:"units"`
			} `json:"targets"`
		}
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		if request.ExpectedRevision == nil || request.AccountIDs == nil || request.Targets == nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "expected_revision, account_ids and targets are required")
			return
		}
		input := app.SaveAllocationInput{ExpectedRevision: *request.ExpectedRevision, AccountIDs: request.AccountIDs, Targets: []db.AllocationTarget{}}
		for _, target := range request.Targets {
			if target.CommodityID == nil || target.Units == nil {
				writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "every target requires commodity_id and units")
				return
			}
			input.Targets = append(input.Targets, db.AllocationTarget{CommodityID: *target.CommodityID, Units: *target.Units})
		}
		input.OwnerUserID, input.AuthSessionID, input.RequestID = owner.ID, authenticatedSessionID(r), RequestIDFromContext(r.Context())
		revision, err := investments.SaveAllocation(r.Context(), input)
		if errors.Is(err, db.ErrAllocationStale) {
			writeAPIError(w, http.StatusConflict, "CONFLICT", "allocation changed; reload before saving")
			return
		}
		if errors.Is(err, db.ErrAllocationReference) {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid allocation account or instrument; existing targets cannot be cleared accidentally")
			return
		}
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "save allocation", err)
			return
		}
		writeJSON(w, http.StatusOK, revision)
	}))
}
