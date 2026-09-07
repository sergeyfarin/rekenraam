package api

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/exact"
)

type forecastQuantityResponse struct {
	QuantityValue exact.Coefficient `json:"quantity_value"`
	QuantityScale int               `json:"quantity_scale"`
}

type forecastCommodityResponse struct {
	ID            int64  `json:"id"`
	Code          string `json:"code"`
	StandardScale int    `json:"standard_scale"`
}

type forecastSourceCountsResponse struct {
	Posted   int `json:"posted"`
	Draft    int `json:"draft"`
	Template int `json:"template"`
}

type forecastAccountResponse struct {
	ID                 int64   `json:"id"`
	ParentAccountID    *int64  `json:"parent_account_id"`
	Name               *string `json:"name"`
	Code               *string `json:"code"`
	BuiltinLabelKey    *string `json:"builtin_label_key"`
	AccountClass       string  `json:"account_class"`
	AccountKind        string  `json:"account_kind"`
	Status             string  `json:"status"`
	AllowsPostings     bool    `json:"allows_postings"`
	DefaultCommodityID *int64  `json:"default_commodity_id"`
}

type forecastPointResponse struct {
	Date                     string                       `json:"date"`
	PostedDelta              forecastQuantityResponse     `json:"posted_delta"`
	DraftDelta               forecastQuantityResponse     `json:"draft_delta"`
	TemplateDelta            forecastQuantityResponse     `json:"template_delta"`
	RecordedBalance          forecastQuantityResponse     `json:"recorded_balance"`
	ProjectedBalance         forecastQuantityResponse     `json:"projected_balance"`
	SourceEventCounts        forecastSourceCountsResponse `json:"source_event_counts"`
	CarriedForwardEventCount int                          `json:"carried_forward_event_count"`
}

type forecastAccountSeriesResponse struct {
	AccountID         int64                    `json:"account_id"`
	CommodityID       int64                    `json:"commodity_id"`
	CommodityCode     string                   `json:"commodity_code"`
	OpeningBalance    forecastQuantityResponse `json:"opening_balance"`
	Points            []forecastPointResponse  `json:"points"`
	MinimumBalance    forecastQuantityResponse `json:"minimum_balance"`
	MinimumDate       string                   `json:"minimum_date"`
	FirstNegativeDate *string                  `json:"first_negative_date"`
}

type forecastCurrencySeriesResponse struct {
	CommodityID    int64                    `json:"commodity_id"`
	CommodityCode  string                   `json:"commodity_code"`
	OpeningBalance forecastQuantityResponse `json:"opening_balance"`
	Points         []forecastPointResponse  `json:"points"`
	MinimumBalance forecastQuantityResponse `json:"minimum_balance"`
	MinimumDate    string                   `json:"minimum_date"`
}

type forecastDiagnosticResponse struct {
	Code           string  `json:"code"`
	Severity       string  `json:"severity"`
	TemplateID     *int64  `json:"template_id"`
	OccurrenceID   *int64  `json:"occurrence_id"`
	TransactionID  *int64  `json:"transaction_id"`
	OccurrenceDate *string `json:"occurrence_date"`
	SourceDate     *string `json:"source_date"`
	ProjectedDate  *string `json:"projected_date"`
	EventCount     int     `json:"event_count"`
}

type forecastScopeResponse struct {
	Mode                string                    `json:"mode"`
	RequestedAccountIDs []int64                   `json:"requested_account_ids"`
	ResolvedAccountIDs  []int64                   `json:"resolved_account_ids"`
	IncludeDescendants  bool                      `json:"include_descendants"`
	Accounts            []forecastAccountResponse `json:"accounts"`
	AccountOptions      []forecastAccountResponse `json:"account_options"`
}

type forecastAssumptionsResponse struct {
	Complete                 bool                         `json:"complete"`
	CarriedForwardEventCount int                          `json:"carried_forward_event_count"`
	ExcludedEventCount       int                          `json:"excluded_event_count"`
	SourceEventCounts        forecastSourceCountsResponse `json:"source_event_counts"`
}

type forecastBalancesResponse struct {
	AsOfDate              string                           `json:"as_of_date"`
	StartDate             string                           `json:"start_date"`
	EndDate               string                           `json:"end_date"`
	TimeZone              string                           `json:"time_zone"`
	ComputedAt            string                           `json:"computed_at"`
	HorizonDays           int                              `json:"horizon_days"`
	BasisToken            string                           `json:"basis_token"`
	PolicyVersion         string                           `json:"policy_version"`
	Scope                 forecastScopeResponse            `json:"scope"`
	CurrencyOptions       []forecastCommodityResponse      `json:"currency_options"`
	Series                []forecastAccountSeriesResponse  `json:"series"`
	Totals                []forecastCurrencySeriesResponse `json:"totals"`
	Assumptions           forecastAssumptionsResponse      `json:"assumptions"`
	Diagnostics           []forecastDiagnosticResponse     `json:"diagnostics"`
	DiagnosticTotalCount  int                              `json:"diagnostic_total_count"`
	DiagnosticHiddenCount int                              `json:"diagnostic_hidden_count"`
	Valuation             *forecastValuationResponse       `json:"valuation"`
	Converted             *forecastCurrencySeriesResponse  `json:"converted"`
}

type forecastRateUseResponse struct {
	ObservationID     int64             `json:"observation_id"`
	BaseCommodityID   int64             `json:"base_commodity_id"`
	QuoteCommodityID  int64             `json:"quote_commodity_id"`
	ValuationDate     string            `json:"valuation_date"`
	RecordedAt        string            `json:"recorded_at"`
	PriceValue        exact.Coefficient `json:"price_value"`
	PriceScale        int               `json:"price_scale"`
	BaseQuantityValue exact.Coefficient `json:"base_quantity_value"`
	BaseQuantityScale int               `json:"base_quantity_scale"`
	IsDerived         bool              `json:"is_derived"`
	Stale             bool              `json:"stale"`
}

type forecastRateGapResponse struct {
	CommodityID            int64   `json:"commodity_id"`
	Reason                 string  `json:"reason"`
	NearestObservationDate *string `json:"nearest_observation_date"`
}

type forecastValuationResponse struct {
	Method                 string                    `json:"method"`
	RateSelection          string                    `json:"rate_selection"`
	AsOfDate               string                    `json:"as_of_date"`
	ReportingCurrencyID    int64                     `json:"reporting_currency_id"`
	ReportingCurrencyCode  string                    `json:"reporting_currency_code"`
	ReportingCurrencyScale int                       `json:"reporting_currency_scale"`
	MaxStalenessDays       int                       `json:"max_staleness_days"`
	Rounding               string                    `json:"rounding"`
	Complete               bool                      `json:"complete"`
	UsedRates              []forecastRateUseResponse `json:"used_rates"`
	Gaps                   []forecastRateGapResponse `json:"gaps"`
}

type forecastEventAmountResponse struct {
	AccountID     int64             `json:"account_id"`
	CommodityID   int64             `json:"commodity_id"`
	CommodityCode string            `json:"commodity_code"`
	QuantityValue exact.Coefficient `json:"quantity_value"`
	QuantityScale int               `json:"quantity_scale"`
}

type forecastEventResponse struct {
	Key            string                        `json:"key"`
	Source         string                        `json:"source"`
	SourceDate     string                        `json:"source_date"`
	ProjectedDate  string                        `json:"projected_date"`
	CarriedForward bool                          `json:"carried_forward"`
	TransactionID  *int64                        `json:"transaction_id"`
	VersionID      *int64                        `json:"version_id"`
	EntryID        *int64                        `json:"entry_id"`
	TemplateID     *int64                        `json:"template_id"`
	OccurrenceID   *int64                        `json:"occurrence_id"`
	OccurrenceDate *string                       `json:"occurrence_date"`
	Description    string                        `json:"description"`
	PayeeName      *string                       `json:"payee_name"`
	Amounts        []forecastEventAmountResponse `json:"amounts"`
}

type forecastEventsResponse struct {
	BasisToken string                  `json:"basis_token"`
	Date       string                  `json:"date"`
	Items      []forecastEventResponse `json:"items"`
	TotalCount int                     `json:"total_count"`
	NextCursor *string                 `json:"next_cursor"`
}

func forecastBalances(logger *slog.Logger, auth *app.AuthService, service *app.ForecastService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		owner, ok := authenticatedOwner(w, r, logger, auth)
		if !ok {
			return
		}
		input, err := parseForecastQuery(r.URL.Query(), false)
		if err != nil {
			writeForecastServiceError(w, r, logger, err)
			return
		}
		input.OwnerUserID = owner.ID
		result, err := service.Balances(r.Context(), input)
		if err != nil {
			writeForecastServiceError(w, r, logger, err)
			return
		}
		writeJSON(w, http.StatusOK, toForecastBalancesResponse(result))
	}
}

func forecastEvents(logger *slog.Logger, auth *app.AuthService, service *app.ForecastService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		owner, ok := authenticatedOwner(w, r, logger, auth)
		if !ok {
			return
		}
		base, err := parseForecastQuery(r.URL.Query(), true)
		if err != nil {
			writeForecastServiceError(w, r, logger, err)
			return
		}
		base.OwnerUserID = owner.ID
		date, err := requiredForecastScalar(r.URL.Query(), "date")
		if err != nil {
			writeForecastServiceError(w, r, logger, err)
			return
		}
		basis, err := requiredForecastScalar(r.URL.Query(), "basis_token")
		if err != nil {
			writeForecastServiceError(w, r, logger, err)
			return
		}
		accountID, err := optionalForecastID(r.URL.Query(), "detail_account_id")
		if err != nil {
			writeForecastServiceError(w, r, logger, err)
			return
		}
		commodityID, err := optionalForecastID(r.URL.Query(), "detail_commodity_id")
		if err != nil {
			writeForecastServiceError(w, r, logger, err)
			return
		}
		limit, err := optionalForecastInt(r.URL.Query(), "limit", app.ForecastDefaultEventLimit)
		if err != nil {
			writeForecastServiceError(w, r, logger, err)
			return
		}
		cursor, err := optionalForecastScalar(r.URL.Query(), "cursor")
		if err != nil {
			writeForecastServiceError(w, r, logger, err)
			return
		}
		result, err := service.Events(r.Context(), app.ForecastEventsInput{ForecastInput: base, Date: date, BasisToken: basis, DetailAccountID: accountID, DetailCommodityID: commodityID, Limit: limit, Cursor: cursor})
		if err != nil {
			writeForecastServiceError(w, r, logger, err)
			return
		}
		writeJSON(w, http.StatusOK, toForecastEventsResponse(result))
	}
}

func parseForecastQuery(query url.Values, events bool) (app.ForecastInput, error) {
	allowed := map[string]bool{"horizon_days": true, "account_id": true, "include_descendants": true, "reporting_currency_id": true, "fx_method": true}
	if events {
		for _, key := range []string{"date", "basis_token", "detail_account_id", "detail_commodity_id", "limit", "cursor"} {
			allowed[key] = true
		}
	}
	for key := range query {
		if !allowed[key] {
			return app.ForecastInput{}, app.ValidationError{Message: "unknown forecast query parameter"}
		}
	}
	horizon, err := optionalForecastInt(query, "horizon_days", app.ForecastDefaultHorizonDays)
	if err != nil {
		return app.ForecastInput{}, err
	}
	if horizon < 1 || horizon > app.ForecastMaxHorizonDays {
		return app.ForecastInput{}, app.ValidationError{Message: "horizon_days must be between 1 and 366"}
	}
	includeDescendants := true
	if values, ok := query["include_descendants"]; ok {
		if len(values) != 1 || (values[0] != "true" && values[0] != "false") {
			return app.ForecastInput{}, app.ValidationError{Message: "include_descendants must be true or false"}
		}
		includeDescendants = values[0] == "true"
	}
	accountIDs := make([]int64, 0, len(query["account_id"]))
	for _, raw := range query["account_id"] {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return app.ForecastInput{}, app.ValidationError{Message: "account_id must be a positive integer"}
		}
		accountIDs = append(accountIDs, id)
	}
	reportingID, err := optionalForecastID(query, "reporting_currency_id")
	if err != nil {
		return app.ForecastInput{}, err
	}
	fxMethod, err := optionalForecastScalar(query, "fx_method")
	if err != nil {
		return app.ForecastInput{}, err
	}
	return app.ForecastInput{HorizonDays: horizon, AccountIDs: accountIDs, IncludeDescendants: includeDescendants, ReportingCurrencyID: reportingID, FXMethod: fxMethod}, nil
}

func optionalForecastInt(query url.Values, key string, fallback int) (int, error) {
	values, ok := query[key]
	if !ok {
		return fallback, nil
	}
	if len(values) != 1 || values[0] == "" {
		return 0, app.ValidationError{Message: key + " must be supplied exactly once"}
	}
	value, err := strconv.ParseInt(values[0], 10, 32)
	if err != nil {
		return 0, app.ValidationError{Message: key + " must be an integer"}
	}
	return int(value), nil
}

func requiredForecastScalar(query url.Values, key string) (string, error) {
	value, err := optionalForecastScalar(query, key)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", app.ValidationError{Message: key + " is required"}
	}
	return value, nil
}

func optionalForecastScalar(query url.Values, key string) (string, error) {
	values, ok := query[key]
	if !ok {
		return "", nil
	}
	if len(values) != 1 || values[0] == "" {
		return "", app.ValidationError{Message: key + " must be supplied exactly once"}
	}
	return values[0], nil
}

func optionalForecastID(query url.Values, key string) (*int64, error) {
	raw, err := optionalForecastScalar(query, key)
	if err != nil || raw == "" {
		return nil, err
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return nil, app.ValidationError{Message: key + " must be a positive integer"}
	}
	return &value, nil
}

func writeForecastServiceError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	var validation app.ValidationError
	var overflow app.LedgerOverflowError
	switch {
	case errors.As(err, &validation):
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", validation.Error())
	case errors.Is(err, app.ErrForecastTooLarge):
		writeAPIError(w, http.StatusUnprocessableEntity, "FORECAST_TOO_LARGE", "forecast exceeds the supported size")
	case errors.As(err, &overflow):
		writeAPIError(w, http.StatusUnprocessableEntity, "LEDGER_OVERFLOW", overflow.Error())
	case errors.Is(err, app.ErrForecastBasisChanged):
		writeAPIError(w, http.StatusConflict, "FORECAST_BASIS_CHANGED", "forecast inputs changed; refresh balances before loading details")
	default:
		writeServiceInternalError(w, r, logger, "read forecast", err)
	}
}

func toForecastBalancesResponse(result app.ForecastResult) forecastBalancesResponse {
	commodities := map[int64]string{}
	for _, commodity := range result.Commodities {
		commodities[commodity.ID] = commodity.Code
	}
	response := forecastBalancesResponse{AsOfDate: result.AsOfDate, StartDate: result.FirstDate, EndDate: result.ThroughDate, TimeZone: result.TimeZone, ComputedAt: result.ComputedAt, HorizonDays: result.HorizonDays, BasisToken: result.BasisToken, PolicyVersion: result.PolicyVersion, Scope: forecastScopeResponse{Mode: result.ScopeMode, RequestedAccountIDs: append([]int64{}, result.RequestedAccountIDs...), ResolvedAccountIDs: append([]int64{}, result.AccountIDs...), IncludeDescendants: result.IncludeDescendants, Accounts: toForecastAccounts(result.Accounts), AccountOptions: toForecastAccounts(result.AccountOptions)}, CurrencyOptions: make([]forecastCommodityResponse, 0, len(result.CurrencyOptions)), Series: make([]forecastAccountSeriesResponse, 0, len(result.Series)), Totals: make([]forecastCurrencySeriesResponse, 0, len(result.Aggregates)), Assumptions: forecastAssumptionsResponse{Complete: result.Assumptions.Complete, CarriedForwardEventCount: result.Assumptions.CarriedForward, ExcludedEventCount: result.Assumptions.Excluded, SourceEventCounts: toForecastSourceCounts(result.SourceCounts)}, Diagnostics: make([]forecastDiagnosticResponse, 0, len(result.Assumptions.Diagnostics)), DiagnosticTotalCount: result.Assumptions.Total, DiagnosticHiddenCount: result.Assumptions.Hidden}
	for _, commodity := range result.CurrencyOptions {
		response.CurrencyOptions = append(response.CurrencyOptions, forecastCommodityResponse{ID: commodity.ID, Code: commodity.Code, StandardScale: commodity.StandardScale})
	}
	for _, series := range result.Series {
		row := forecastAccountSeriesResponse{AccountID: series.AccountID, CommodityID: series.CommodityID, CommodityCode: commodities[series.CommodityID], OpeningBalance: toForecastQuantity(series.Opening), Points: toForecastPoints(series.Points), MinimumBalance: toForecastQuantity(series.Minimum), MinimumDate: series.MinimumDate, FirstNegativeDate: optionalString(series.FirstNegativeDate)}
		response.Series = append(response.Series, row)
	}
	for _, series := range result.Aggregates {
		response.Totals = append(response.Totals, forecastCurrencySeriesResponse{CommodityID: series.CommodityID, CommodityCode: commodities[series.CommodityID], OpeningBalance: toForecastQuantity(series.Opening), Points: toForecastPoints(series.Points), MinimumBalance: toForecastQuantity(series.Minimum), MinimumDate: series.MinimumDate})
	}
	for _, diagnostic := range result.Assumptions.Diagnostics {
		response.Diagnostics = append(response.Diagnostics, forecastDiagnosticResponse{Code: diagnostic.Code, Severity: diagnostic.Severity, TemplateID: optionalInt64(diagnostic.TemplateID), OccurrenceID: optionalInt64(diagnostic.OccurrenceID), TransactionID: optionalInt64(diagnostic.TransactionID), OccurrenceDate: optionalString(diagnostic.OccurrenceDate), SourceDate: optionalString(diagnostic.SourceDate), ProjectedDate: optionalString(diagnostic.ProjectedDate), EventCount: diagnostic.EventCount})
	}
	if result.Valuation != nil {
		response.Valuation = toForecastValuationResponse(*result.Valuation)
	}
	if result.Converted != nil {
		series := result.Converted
		response.Converted = &forecastCurrencySeriesResponse{CommodityID: series.CommodityID, CommodityCode: result.Valuation.ReportingCurrencyCode, OpeningBalance: toForecastQuantity(series.Opening), Points: toForecastPoints(series.Points), MinimumBalance: toForecastQuantity(series.Minimum), MinimumDate: series.MinimumDate}
	}
	return response
}

func toForecastValuationResponse(input app.ForecastValuation) *forecastValuationResponse {
	result := &forecastValuationResponse{Method: input.Method, RateSelection: input.RateSelection, AsOfDate: input.AsOfDate, ReportingCurrencyID: input.ReportingCurrencyID, ReportingCurrencyCode: input.ReportingCurrencyCode, ReportingCurrencyScale: input.ReportingCurrencyScale, MaxStalenessDays: input.MaxStalenessDays, Rounding: input.Rounding, Complete: input.Complete, UsedRates: make([]forecastRateUseResponse, 0, len(input.UsedRates)), Gaps: make([]forecastRateGapResponse, 0, len(input.Gaps))}
	for _, rate := range input.UsedRates {
		result.UsedRates = append(result.UsedRates, forecastRateUseResponse{ObservationID: rate.ObservationID, BaseCommodityID: rate.BaseCommodityID, QuoteCommodityID: rate.QuoteCommodityID, ValuationDate: rate.ValuationDate, RecordedAt: rate.RecordedAt, PriceValue: rate.PriceValue, PriceScale: rate.PriceScale, BaseQuantityValue: rate.BaseQuantityValue, BaseQuantityScale: rate.BaseQuantityScale, IsDerived: rate.IsDerived, Stale: rate.Stale})
	}
	for _, gap := range input.Gaps {
		result.Gaps = append(result.Gaps, forecastRateGapResponse{CommodityID: gap.CommodityID, Reason: gap.Reason, NearestObservationDate: optionalString(gap.NearestObservationDate)})
	}
	return result
}

func toForecastAccounts(input []app.ForecastAccount) []forecastAccountResponse {
	result := make([]forecastAccountResponse, 0, len(input))
	for _, account := range input {
		result = append(result, forecastAccountResponse{ID: account.ID, ParentAccountID: account.ParentAccountID, Name: optionalString(account.Name), Code: optionalString(account.Code), BuiltinLabelKey: optionalString(account.BuiltinLabelKey), AccountClass: account.AccountClass, AccountKind: account.AccountKind, Status: account.Status, AllowsPostings: account.AllowsPostings, DefaultCommodityID: account.DefaultCommodityID})
	}
	return result
}

func toForecastPoints(input []app.ForecastPoint) []forecastPointResponse {
	result := make([]forecastPointResponse, 0, len(input))
	for _, point := range input {
		result = append(result, forecastPointResponse{Date: point.Date, PostedDelta: toForecastQuantity(point.Components.Posted), DraftDelta: toForecastQuantity(point.Components.Draft), TemplateDelta: toForecastQuantity(point.Components.Template), RecordedBalance: toForecastQuantity(point.RecordedBalance), ProjectedBalance: toForecastQuantity(point.ProjectedBalance), SourceEventCounts: toForecastSourceCounts(point.SourceCounts), CarriedForwardEventCount: point.CarriedForward})
	}
	return result
}

func toForecastQuantity(input app.ForecastQuantity) forecastQuantityResponse {
	return forecastQuantityResponse{QuantityValue: input.Value, QuantityScale: input.Scale}
}

func toForecastSourceCounts(input app.ForecastSourceCounts) forecastSourceCountsResponse {
	return forecastSourceCountsResponse{Posted: input.Posted, Draft: input.Draft, Template: input.Template}
}

func toForecastEventsResponse(result app.ForecastEventsResult) forecastEventsResponse {
	response := forecastEventsResponse{BasisToken: result.BasisToken, Date: result.Date, Items: make([]forecastEventResponse, 0, len(result.Items)), TotalCount: result.TotalCount, NextCursor: optionalString(result.NextCursor)}
	for _, event := range result.Items {
		row := forecastEventResponse{Key: event.Key, Source: event.Source, SourceDate: event.SourceDate, ProjectedDate: event.ProjectedDate, CarriedForward: event.CarriedForward, Description: event.Description, PayeeName: optionalString(event.PayeeName), Amounts: make([]forecastEventAmountResponse, 0, len(event.Amounts))}
		if event.Source == "posted" || event.Source == "draft" {
			row.TransactionID, row.VersionID, row.EntryID = optionalInt64(event.TransactionID), optionalInt64(event.VersionID), optionalInt64(event.EntryID)
		}
		if event.Source == "draft" || event.Source == "template" {
			row.TemplateID, row.OccurrenceDate = optionalInt64(event.TemplateID), optionalString(event.OccurrenceDate)
		}
		if event.Source == "draft" {
			row.OccurrenceID = optionalInt64(event.OccurrenceID)
		}
		for _, amount := range event.Amounts {
			row.Amounts = append(row.Amounts, forecastEventAmountResponse{AccountID: amount.AccountID, CommodityID: amount.CommodityID, CommodityCode: amount.CommodityCode, QuantityValue: amount.Quantity.Value, QuantityScale: amount.Quantity.Scale})
		}
		response.Items = append(response.Items, row)
	}
	return response
}

func optionalInt64(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
