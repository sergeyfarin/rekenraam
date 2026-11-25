package api

import (
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
)

type planningOverviewResponse struct {
	AsOf      string                                      `json:"as_of"`
	Budget    readCardResponse[planningBudgetResponse]    `json:"budget"`
	Recurring readCardResponse[planningRecurringResponse] `json:"recurring"`
	Forecast  readCardResponse[planningForecastResponse]  `json:"forecast"`
}

type planningBudgetResponse struct {
	StartDate          string                        `json:"start_date"`
	EndDate            string                        `json:"end_date"`
	ElapsedBasisPoints int                           `json:"elapsed_basis_points"`
	Totals             []planningBudgetTotalResponse `json:"totals"`
}

type planningBudgetTotalResponse struct {
	CommodityID      int64              `json:"commodity_id"`
	Remaining        app.OverviewAmount `json:"remaining"`
	SpentBasisPoints string             `json:"spent_basis_points"`
}

type planningRecurringResponse struct {
	DraftCount   int `json:"draft_count"`
	BlockedCount int `json:"blocked_count"`
}

type planningForecastResponse struct {
	ThroughDate string                `json:"through_date"`
	HorizonDays int                   `json:"horizon_days"`
	Complete    bool                  `json:"complete"`
	Minimums    []app.PlanningMinimum `json:"minimums"`
}

type reportsOverviewResponse struct {
	AsOf                string                                      `json:"as_of"`
	MonthStart          string                                      `json:"month_start"`
	ReportingCurrencyID *int64                                      `json:"reporting_currency_id"`
	Spending            readCardResponse[app.ReportHeadline]        `json:"spending"`
	Cashflow            readCardResponse[app.ReportHeadline]        `json:"cashflow"`
	NetWorth            readCardResponse[app.ReportHeadline]        `json:"net_worth"`
	GainsRuns           readCardResponse[[]app.OverviewSavedReport] `json:"gains_runs"`
	ReturnsRuns         readCardResponse[[]app.OverviewSavedReport] `json:"returns_runs"`
}

func planningOverview(logger *slog.Logger, services Services) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, services.Auth)
		if !ok {
			return
		}
		data, err := app.ReadPlanningOverview(r.Context(), services.Settings, services.Budget, services.Recurring, services.Forecast, owner.ID)
		if err != nil {
			writeServiceInternalError(w, r, logger, "read planning overview", err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, planningOverviewResponse{
			AsOf:      data.AsOf,
			Budget:    mapReadCard(r, logger, "planning: budgets", data.Budget, toPlanningBudget),
			Recurring: mapReadCard(r, logger, "planning: recurring", data.Recurring, toPlanningRecurring),
			Forecast:  mapReadCard(r, logger, "planning: forecast", data.Forecast, toPlanningForecast),
		})
	}
}

func reportsOverview(logger *slog.Logger, services Services) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, services.Auth); !ok {
			return
		}
		data, err := app.ReadReportsOverview(r.Context(), services.Transaction, services.Investment)
		if err != nil {
			writeServiceInternalError(w, r, logger, "read reports overview", err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, reportsOverviewResponse{
			AsOf:                data.AsOf,
			MonthStart:          data.MonthStart,
			ReportingCurrencyID: data.ReportingCurrencyID,
			Spending:            toReadCard(r, logger, "reports: spending", data.Spending),
			Cashflow:            toReadCard(r, logger, "reports: cashflow", data.Cashflow),
			NetWorth:            toReadCard(r, logger, "reports: net worth", data.NetWorth),
			GainsRuns:           toReadCard(r, logger, "reports: saved gains", data.GainsRuns),
			ReturnsRuns:         toReadCard(r, logger, "reports: saved returns", data.ReturnsRuns),
		})
	}
}

func toPlanningBudget(b app.PlanningBudget) planningBudgetResponse {
	totals := make([]planningBudgetTotalResponse, 0, len(b.Totals))
	for _, total := range b.Totals {
		totals = append(totals, planningBudgetTotalResponse(total))
	}
	return planningBudgetResponse{StartDate: b.StartDate, EndDate: b.EndDate, ElapsedBasisPoints: b.ElapsedBasisPoints, Totals: totals}
}

func toPlanningRecurring(r app.RecurringSummary) planningRecurringResponse {
	return planningRecurringResponse{DraftCount: r.DraftCount, BlockedCount: r.BlockedCount}
}

func toPlanningForecast(f app.PlanningForecast) planningForecastResponse {
	return planningForecastResponse(f)
}
