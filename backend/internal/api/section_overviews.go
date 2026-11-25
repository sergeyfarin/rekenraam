package api

import (
	"fmt"
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
)

type sectionOverviewCard[T any] struct {
	Data   *T   `json:"data"`
	Failed bool `json:"failed"`
}

type planningOverviewResponse struct {
	AsOf      string                                         `json:"as_of"`
	Budget    sectionOverviewCard[planningBudgetResponse]    `json:"budget"`
	Recurring sectionOverviewCard[planningRecurringResponse] `json:"recurring"`
	Forecast  sectionOverviewCard[planningForecastResponse]  `json:"forecast"`
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
	AsOf                string                                         `json:"as_of"`
	MonthStart          string                                         `json:"month_start"`
	ReportingCurrencyID *int64                                         `json:"reporting_currency_id"`
	Spending            sectionOverviewCard[app.ReportHeadline]        `json:"spending"`
	Cashflow            sectionOverviewCard[app.ReportHeadline]        `json:"cashflow"`
	NetWorth            sectionOverviewCard[app.ReportHeadline]        `json:"net_worth"`
	GainsRuns           sectionOverviewCard[[]app.OverviewSavedReport] `json:"gains_runs"`
	ReturnsRuns         sectionOverviewCard[[]app.OverviewSavedReport] `json:"returns_runs"`
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
			Budget:    mapOverviewCard(r, logger, "budgets", data.Budget, toPlanningBudget),
			Recurring: mapOverviewCard(r, logger, "recurring", data.Recurring, toPlanningRecurring),
			Forecast:  mapOverviewCard(r, logger, "forecast", data.Forecast, toPlanningForecast),
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
			Spending:            overviewCard(r, logger, "spending", data.Spending),
			Cashflow:            overviewCard(r, logger, "cashflow", data.Cashflow),
			NetWorth:            overviewCard(r, logger, "net worth", data.NetWorth),
			GainsRuns:           overviewCard(r, logger, "saved gains", data.GainsRuns),
			ReturnsRuns:         overviewCard(r, logger, "saved returns", data.ReturnsRuns),
		})
	}
}

func overviewCard[T any](r *http.Request, logger *slog.Logger, source string, result app.OverviewSource[T]) sectionOverviewCard[T] {
	if result.Err != nil {
		// A dependency error may carry financial content. Record the source
		// and error type so the response can safely keep the other cards.
		logger.ErrorContext(r.Context(), "read section overview card", slog.String("source", source), slog.String("error_type", fmt.Sprintf("%T", result.Err)))
		return sectionOverviewCard[T]{Failed: true}
	}
	return sectionOverviewCard[T]{Data: &result.Data}
}

func mapOverviewCard[T, R any](r *http.Request, logger *slog.Logger, source string, result app.OverviewSource[T], convert func(T) R) sectionOverviewCard[R] {
	if result.Err != nil {
		return overviewCard(r, logger, source, app.OverviewSource[R]{Err: result.Err})
	}
	return overviewCard(r, logger, source, app.OverviewSource[R]{Data: convert(result.Data)})
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
