package api

import (
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
)

type homeReportingCurrencyResponse struct {
	CommodityID int64  `json:"commodity_id"`
	Code        string `json:"code"`
	Kind        string `json:"kind"`
	Symbol      string `json:"symbol"`
	Scale       int    `json:"scale"`
}

type homeValuationInputResponse struct {
	CommodityID int64  `json:"commodity_id"`
	Code        string `json:"code"`
	Kind        string `json:"kind"`
}

type homeHeadlineResponse struct {
	ID           string                       `json:"id"`
	AccountCount int                          `json:"account_count"`
	Value        *balanceQuantityResponse     `json:"value"`
	StartValue   *balanceQuantityResponse     `json:"start_value"`
	Change       *balanceQuantityResponse     `json:"change"`
	Unavailable  *string                      `json:"unavailable"`
	Missing      []homeValuationInputResponse `json:"missing"`
	Stale        []homeValuationInputResponse `json:"stale"`
}

type homeResponse struct {
	AsOf              string                          `json:"as_of"`
	StartBoundary     string                          `json:"start_boundary"`
	ReportingCurrency *homeReportingCurrencyResponse  `json:"reporting_currency"`
	EmptyBook         bool                            `json:"empty_book"`
	Headlines         []homeHeadlineResponse          `json:"headlines"`
	Widgets           homeWidgetsResponse             `json:"widgets"`
	Commodities       []homeReportingCurrencyResponse `json:"commodities"`
}

type homeWidgetResponse[T any] struct {
	Status  string `json:"status"`
	Summary *T     `json:"summary"`
}

type homeWidgetsResponse struct {
	Spending      homeWidgetResponse[homeSpendingSummaryResponse]      `json:"spending"`
	Budgets       homeWidgetResponse[homeBudgetSummaryResponse]        `json:"budgets"`
	Accounts      homeWidgetResponse[homeAccountsSummaryResponse]      `json:"accounts"`
	Recurring     homeWidgetResponse[homeRecurringSummaryResponse]     `json:"recurring"`
	NetWorthTrend homeWidgetResponse[homeNetWorthTrendSummaryResponse] `json:"net_worth_trend"`
}

type homeSpendingSummaryResponse struct {
	StartDate       string                         `json:"start_date"`
	EndDate         string                         `json:"end_date"`
	Total           *balanceQuantityResponse       `json:"total"`
	CommodityTotals []balanceQuantityResponse      `json:"commodity_totals"`
	Categories      []homeSpendingCategoryResponse `json:"categories"`
	CategoryCount   int                            `json:"category_count"`
}

type homeSpendingCategoryResponse struct {
	CategoryID int64                     `json:"category_id"`
	Code       string                    `json:"code"`
	Name       string                    `json:"name"`
	BuiltinKey *string                   `json:"builtin_key"`
	Converted  *balanceQuantityResponse  `json:"converted"`
	Totals     []balanceQuantityResponse `json:"totals"`
}

type homeBudgetSummaryResponse struct {
	PeriodStart         string                        `json:"period_start"`
	PeriodEnd           string                        `json:"period_end"`
	CommodityID         *int64                        `json:"commodity_id"`
	OtherCommodityCount int                           `json:"other_commodity_count"`
	Total               *budgetCategoryAmountResponse `json:"total"`
	Categories          []homeBudgetCategoryResponse  `json:"categories"`
	CategoryCount       int                           `json:"category_count"`
}

type homeBudgetCategoryResponse struct {
	CategoryID int64                        `json:"category_id"`
	Code       string                       `json:"code"`
	Name       string                       `json:"name"`
	BuiltinKey *string                      `json:"builtin_key"`
	Amount     budgetCategoryAmountResponse `json:"amount"`
}

type homeAccountsSummaryResponse struct {
	AsOf      string                   `json:"as_of"`
	OpenCount int                      `json:"open_count"`
	Accounts  []homeAccountRowResponse `json:"accounts"`
}

type homeAccountRowResponse struct {
	AccountID    int64                     `json:"account_id"`
	Code         string                    `json:"code"`
	Name         string                    `json:"name"`
	AccountClass string                    `json:"account_class"`
	AccountKind  string                    `json:"account_kind"`
	Balances     []balanceQuantityResponse `json:"balances"`
	Value        *balanceQuantityResponse  `json:"value"`
}

type homeRecurringSummaryResponse struct {
	Today        string                      `json:"today"`
	ActiveCount  int                         `json:"active_count"`
	DraftCount   int                         `json:"draft_count"`
	BlockedCount int                         `json:"blocked_count"`
	Upcoming     []homeRecurringItemResponse `json:"upcoming"`
}

type homeRecurringItemResponse struct {
	TemplateID int64                     `json:"template_id"`
	Name       string                    `json:"name"`
	NextDueOn  string                    `json:"next_due_on"`
	Amounts    []balanceQuantityResponse `json:"amounts"`
}

type homeNetWorthTrendSummaryResponse struct {
	StartDate string                   `json:"start_date"`
	EndDate   string                   `json:"end_date"`
	Points    []homeTrendPointResponse `json:"points"`
	Change    *balanceQuantityResponse `json:"change"`
}

type homeTrendPointResponse struct {
	Date  string                   `json:"date"`
	Value *balanceQuantityResponse `json:"value"`
}

func homeRead(logger *slog.Logger, authService *app.AuthService, homeService *app.HomeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		home, err := homeService.Home(r.Context(), owner.ID)
		if err != nil {
			writeLedgerServiceError(w, r, logger, "read home", err)
			return
		}
		result := home.Briefing
		response := homeResponse{AsOf: result.AsOf, StartBoundary: result.StartBoundary, EmptyBook: result.EmptyBook, Headlines: []homeHeadlineResponse{}, Commodities: []homeReportingCurrencyResponse{}}
		if currency := result.ReportingCurrency; currency != nil {
			response.ReportingCurrency = &homeReportingCurrencyResponse{CommodityID: currency.ID, Code: currency.Code, Kind: currency.Kind, Symbol: currency.DisplaySymbol, Scale: currency.StandardScale}
		}
		for _, commodity := range home.Commodities {
			response.Commodities = append(response.Commodities, homeReportingCurrencyResponse{CommodityID: commodity.ID, Code: commodity.Code, Kind: commodity.Kind, Symbol: commodity.DisplaySymbol, Scale: commodity.StandardScale})
		}
		response.Widgets = homeWidgetsResponse{
			Spending:      toHomeWidget(r, logger, "spending", home.Spending, toHomeSpendingSummary),
			Budgets:       toHomeWidget(r, logger, "budgets", home.Budgets, toHomeBudgetSummary),
			Accounts:      toHomeWidget(r, logger, "accounts", home.Accounts, toHomeAccountsSummary),
			Recurring:     toHomeWidget(r, logger, "recurring", home.Recurring, toHomeRecurringSummary),
			NetWorthTrend: toHomeWidget(r, logger, "net_worth_trend", home.NetWorthTrend, toHomeNetWorthTrendSummary),
		}
		for _, headline := range result.Headlines {
			item := homeHeadlineResponse{
				ID:           headline.ID,
				AccountCount: headline.AccountCount,
				Value:        toBalanceQuantityPointer(headline.Value),
				StartValue:   toBalanceQuantityPointer(headline.StartValue),
				Change:       toBalanceQuantityPointer(headline.Change),
				Missing:      toHomeValuationInputs(headline.Missing),
				Stale:        toHomeValuationInputs(headline.Stale),
			}
			if headline.Unavailable != "" {
				item.Unavailable = &headline.Unavailable
			}
			response.Headlines = append(response.Headlines, item)
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, response)
	}
}

func toHomeValuationInputs(inputs []app.HomeValuationInput) []homeValuationInputResponse {
	response := make([]homeValuationInputResponse, 0, len(inputs))
	for _, input := range inputs {
		response = append(response, homeValuationInputResponse{CommodityID: input.CommodityID, Code: input.Code, Kind: input.Kind})
	}
	return response
}

// toHomeWidget maps one summary, or logs why it failed and reports it failed:
// the widget's failure stays in its card.
func toHomeWidget[T any, R any](r *http.Request, logger *slog.Logger, widget string, result app.HomeWidget[T], convert func(T) R) homeWidgetResponse[R] {
	if result.Err != nil || result.Summary == nil {
		logger.ErrorContext(r.Context(), "read home widget", slog.String("widget", widget), slog.Any("err", result.Err))
		return homeWidgetResponse[R]{Status: "failed"}
	}
	summary := convert(*result.Summary)
	return homeWidgetResponse[R]{Status: "ok", Summary: &summary}
}

func toHomeSpendingSummary(summary app.HomeSpendingSummary) homeSpendingSummaryResponse {
	response := homeSpendingSummaryResponse{StartDate: summary.StartDate, EndDate: summary.EndDate, Total: toBalanceQuantityPointer(summary.Total),
		CommodityTotals: toBalanceQuantityResponses(summary.CommodityTotals), Categories: []homeSpendingCategoryResponse{}, CategoryCount: summary.CategoryCount}
	for _, category := range summary.Categories {
		response.Categories = append(response.Categories, homeSpendingCategoryResponse{CategoryID: category.CategoryID, Code: category.Code, Name: category.Name, BuiltinKey: optionalHomeString(category.BuiltinKey),
			Converted: toBalanceQuantityPointer(category.Converted), Totals: toBalanceQuantityResponses(category.Totals)})
	}
	return response
}

func toHomeBudgetSummary(summary app.HomeBudgetSummary) homeBudgetSummaryResponse {
	response := homeBudgetSummaryResponse{PeriodStart: summary.PeriodStart, PeriodEnd: summary.PeriodEnd, OtherCommodityCount: summary.OtherCommodityCount,
		Categories: []homeBudgetCategoryResponse{}, CategoryCount: summary.CategoryCount}
	if summary.CommodityID != 0 {
		id := summary.CommodityID
		response.CommodityID = &id
	}
	if summary.Total != nil {
		total := toBudgetCategoryAmount(*summary.Total)
		response.Total = &total
	}
	for _, category := range summary.Categories {
		response.Categories = append(response.Categories, homeBudgetCategoryResponse{CategoryID: category.CategoryID, Code: category.Code, Name: category.Name, BuiltinKey: optionalHomeString(category.BuiltinKey), Amount: toBudgetCategoryAmount(category.Amount)})
	}
	return response
}

func toHomeAccountsSummary(summary app.HomeAccountsSummary) homeAccountsSummaryResponse {
	response := homeAccountsSummaryResponse{AsOf: summary.AsOf, OpenCount: summary.OpenCount, Accounts: []homeAccountRowResponse{}}
	for _, row := range summary.Accounts {
		response.Accounts = append(response.Accounts, homeAccountRowResponse{AccountID: row.AccountID, Code: row.Code, Name: row.Name, AccountClass: row.AccountClass,
			AccountKind: row.AccountKind, Balances: toBalanceQuantityResponses(row.Balances), Value: toBalanceQuantityPointer(row.Value)})
	}
	return response
}

func toHomeRecurringSummary(summary app.HomeRecurringSummary) homeRecurringSummaryResponse {
	response := homeRecurringSummaryResponse{Today: summary.Today, ActiveCount: summary.ActiveCount, DraftCount: summary.DraftCount, BlockedCount: summary.BlockedCount, Upcoming: []homeRecurringItemResponse{}}
	for _, item := range summary.Upcoming {
		response.Upcoming = append(response.Upcoming, homeRecurringItemResponse{TemplateID: item.TemplateID, Name: item.Name, NextDueOn: item.NextDueOn, Amounts: toBalanceQuantityResponses(item.Amounts)})
	}
	return response
}

func toHomeNetWorthTrendSummary(summary app.HomeNetWorthTrendSummary) homeNetWorthTrendSummaryResponse {
	response := homeNetWorthTrendSummaryResponse{StartDate: summary.StartDate, EndDate: summary.EndDate, Points: []homeTrendPointResponse{}, Change: toBalanceQuantityPointer(summary.Change)}
	for _, point := range summary.Points {
		response.Points = append(response.Points, homeTrendPointResponse{Date: point.Date, Value: toBalanceQuantityPointer(point.Value)})
	}
	return response
}

func optionalHomeString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
