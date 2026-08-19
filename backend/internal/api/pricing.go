package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"rekenraam/backend/internal/app"
)

type marketDataSourceResponse struct {
	ID          int64           `json:"id"`
	Code        string          `json:"code"`
	Name        string          `json:"name"`
	Kind        string          `json:"kind"`
	ProviderKey string          `json:"provider_key,omitempty"`
	BaseURL     string          `json:"base_url,omitempty"`
	Status      string          `json:"status"`
	Metadata    json.RawMessage `json:"metadata"`
	CreatedAt   string          `json:"created_at"`
}

type marketDataSourcesResponse struct {
	Sources []marketDataSourceResponse `json:"sources"`
}

type priceObservationResponse struct {
	ID                      int64           `json:"id"`
	BookID                  int64           `json:"book_id"`
	SeriesID                int64           `json:"series_id"`
	BaseCommodityID         int64           `json:"base_commodity_id"`
	QuoteCommodityID        int64           `json:"quote_commodity_id"`
	QuoteType               string          `json:"quote_type"`
	AdjustmentBasis         string          `json:"adjustment_basis"`
	PriceValue              int64           `json:"price_value"`
	PriceScale              int             `json:"price_scale"`
	BaseQuantityValue       int64           `json:"base_quantity_value"`
	BaseQuantityScale       int             `json:"base_quantity_scale"`
	ValuationDate           string          `json:"valuation_date"`
	ObservedAt              string          `json:"observed_at,omitempty"`
	SourcePublishedAt       string          `json:"source_published_at,omitempty"`
	SourceID                *int64          `json:"source_id,omitempty"`
	ProviderObservationID   string          `json:"provider_observation_id,omitempty"`
	IsManual                bool            `json:"is_manual"`
	IsDerived               bool            `json:"is_derived"`
	SupersedesObservationID *int64          `json:"supersedes_observation_id,omitempty"`
	Derivation              json.RawMessage `json:"derivation"`
	Metadata                json.RawMessage `json:"metadata"`
	RecordedAt              string          `json:"recorded_at"`
	VoidedAt                string          `json:"voided_at,omitempty"`
	VoidReason              string          `json:"void_reason,omitempty"`
}

type priceObservationsResponse struct {
	Prices []priceObservationResponse `json:"prices"`
}

type priceObservationRequest struct {
	BaseCommodityID         int64           `json:"base_commodity_id"`
	CommodityID             int64           `json:"commodity_id"`
	QuoteCommodityID        int64           `json:"quote_commodity_id"`
	QuoteType               string          `json:"quote_type"`
	AdjustmentBasis         string          `json:"adjustment_basis"`
	PriceValue              int64           `json:"price_value"`
	PriceScale              int             `json:"price_scale"`
	BaseQuantityValue       int64           `json:"base_quantity_value"`
	BaseQuantityScale       int             `json:"base_quantity_scale"`
	ValuationDate           string          `json:"valuation_date"`
	PriceDate               string          `json:"price_date"`
	ObservedAt              string          `json:"observed_at"`
	SourcePublishedAt       string          `json:"source_published_at"`
	SourceID                *int64          `json:"source_id"`
	MarketCode              string          `json:"market_code"`
	ProviderSeriesID        string          `json:"provider_series_id"`
	ProviderObservationID   string          `json:"provider_observation_id"`
	IsManual                bool            `json:"is_manual"`
	IsDerived               bool            `json:"is_derived"`
	SupersedesObservationID *int64          `json:"supersedes_observation_id"`
	Derivation              json.RawMessage `json:"derivation"`
	SeriesMetadata          json.RawMessage `json:"series_metadata"`
	Metadata                json.RawMessage `json:"metadata"`
	ChangeReason            string          `json:"change_reason"`
}

type pricingPolicyResponse struct {
	ID                   int64  `json:"id"`
	BookID               int64  `json:"book_id"`
	BaseCommodityID      *int64 `json:"base_commodity_id,omitempty"`
	DefaultSourceID      *int64 `json:"default_source_id,omitempty"`
	RefreshEnabled       bool   `json:"refresh_enabled"`
	RefreshHourUTC       int    `json:"refresh_hour_utc"`
	RefreshMinuteUTC     int    `json:"refresh_minute_utc"`
	RefreshHourLocal     int    `json:"refresh_hour_local"`
	RefreshMinuteLocal   int    `json:"refresh_minute_local"`
	MaxBackfillDays      int    `json:"max_backfill_days"`
	StalenessMaxDays     int    `json:"staleness_max_days"`
	TriangulationMaxHops int    `json:"triangulation_max_hops"`
	RoundingMode         string `json:"rounding_mode"`
	PreferOfficialFX     bool   `json:"prefer_official_fx"`
	WeekendPolicy        string `json:"weekend_policy"`
	CreatedAt            string `json:"created_at"`
	UpdatedAt            string `json:"updated_at"`
}

type pricingPolicyRequest struct {
	BaseCommodityID      *int64 `json:"base_commodity_id"`
	DefaultSourceID      *int64 `json:"default_source_id"`
	RefreshEnabled       bool   `json:"refresh_enabled"`
	RefreshHourUTC       int    `json:"refresh_hour_utc"`
	RefreshMinuteUTC     int    `json:"refresh_minute_utc"`
	RefreshHourLocal     *int   `json:"refresh_hour_local,omitempty"`
	RefreshMinuteLocal   *int   `json:"refresh_minute_local,omitempty"`
	MaxBackfillDays      int    `json:"max_backfill_days"`
	StalenessMaxDays     int    `json:"staleness_max_days"`
	TriangulationMaxHops int    `json:"triangulation_max_hops"`
	RoundingMode         string `json:"rounding_mode"`
	PreferOfficialFX     bool   `json:"prefer_official_fx"`
	WeekendPolicy        string `json:"weekend_policy"`
	ChangeReason         string `json:"change_reason"`
}

type pricingSourceAssignmentResponse struct {
	ID               int64  `json:"id"`
	BookID           int64  `json:"book_id"`
	CommodityID      int64  `json:"commodity_id"`
	QuoteCommodityID int64  `json:"quote_commodity_id"`
	SourceID         int64  `json:"source_id"`
	Priority         int    `json:"priority"`
	Status           string `json:"status"`
	EffectiveFrom    string `json:"effective_from"`
	EffectiveTo      string `json:"effective_to,omitempty"`
	CreatedAt        string `json:"created_at"`
}

type pricingSourceAssignmentsResponse struct {
	Assignments []pricingSourceAssignmentResponse `json:"assignments"`
}

type pricingSourceAssignmentRequest struct {
	CommodityID      int64  `json:"commodity_id"`
	QuoteCommodityID int64  `json:"quote_commodity_id"`
	SourceID         int64  `json:"source_id"`
	Priority         int    `json:"priority"`
	Status           string `json:"status"`
	EffectiveFrom    string `json:"effective_from"`
	EffectiveTo      string `json:"effective_to"`
	ChangeReason     string `json:"change_reason"`
}

type pricingRefreshRunResponse struct {
	ID             int64  `json:"id"`
	BookID         int64  `json:"book_id"`
	SourceID       int64  `json:"source_id"`
	Trigger        string `json:"trigger"`
	Status         string `json:"status"`
	StartedAt      string `json:"started_at"`
	FinishedAt     string `json:"finished_at,omitempty"`
	ItemsTotal     int    `json:"items_total"`
	ItemsSucceeded int    `json:"items_succeeded"`
	ItemsFailed    int    `json:"items_failed"`
	LastError      string `json:"last_error,omitempty"`
}

type pricingRefreshRunsResponse struct {
	Runs    []pricingRefreshRunResponse `json:"runs"`
	HasMore bool                        `json:"has_more"`
}

type pricingRefreshRunRequest struct {
	SourceID int64  `json:"source_id"`
	Trigger  string `json:"trigger"`
}

type pricingSourceHealthResponse struct {
	ID               int64  `json:"id"`
	BookID           int64  `json:"book_id"`
	CommodityID      int64  `json:"commodity_id"`
	QuoteCommodityID int64  `json:"quote_commodity_id"`
	SourceID         int64  `json:"source_id"`
	LastSuccessDate  string `json:"last_success_date,omitempty"`
	LastAttemptAt    string `json:"last_attempt_at,omitempty"`
	LastError        string `json:"last_error,omitempty"`
	UpdatedAt        string `json:"updated_at"`
	Status           string `json:"status"`
}

type pricingBackgroundWorkResponse struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`
	Reason    string `json:"reason,omitempty"`
	Attempts  int    `json:"attempts"`
	LastError string `json:"last_error,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type pricingSourceHealthListResponse struct {
	Sources []pricingSourceHealthResponse `json:"sources"`
	// FailedBackgroundWork is FX coverage work that exhausted its retries and
	// needs a manual re-enqueue (T-39).
	FailedBackgroundWork []pricingBackgroundWorkResponse `json:"failed_background_work"`
}

func listMarketDataSources(logger *slog.Logger, authService *app.AuthService, pricingService *app.PricingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		sources, err := pricingService.ListSources(r.Context())
		if err != nil {
			writePricingServiceError(w, r, logger, "list market data sources", err)
			return
		}
		writeJSON(w, http.StatusOK, marketDataSourcesResponse{Sources: toMarketDataSourceResponses(sources)})
	}
}

func listPrices(logger *slog.Logger, authService *app.AuthService, pricingService *app.PricingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		baseCommodityValue := r.URL.Query().Get("base_commodity_id")
		if baseCommodityValue == "" {
			baseCommodityValue = r.URL.Query().Get("commodity_id")
		}
		baseCommodityID, ok := parseOptionalPositiveInt64(w, baseCommodityValue, "base commodity id")
		if !ok {
			return
		}
		quoteCommodityID, ok := parseOptionalPositiveInt64(w, r.URL.Query().Get("quote_commodity_id"), "quote commodity id")
		if !ok {
			return
		}
		limit := 100
		if r.URL.Query().Get("limit") != "" {
			parsed, err := strconv.Atoi(r.URL.Query().Get("limit"))
			if err != nil || parsed <= 0 {
				writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "limit is invalid")
				return
			}
			limit = parsed
		}
		prices, err := pricingService.ListPrices(r.Context(), baseCommodityID, quoteCommodityID, limit)
		if err != nil {
			writePricingServiceError(w, r, logger, "list prices", err)
			return
		}
		writeJSON(w, http.StatusOK, priceObservationsResponse{Prices: toPriceObservationResponses(prices)})
	}
}

func createPrice(logger *slog.Logger, authService *app.AuthService, pricingService *app.PricingService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request priceObservationRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		baseCommodityID := request.BaseCommodityID
		if baseCommodityID == 0 {
			baseCommodityID = request.CommodityID
		}
		valuationDate := request.ValuationDate
		if valuationDate == "" {
			valuationDate = request.PriceDate
		}
		price, err := pricingService.CreatePrice(r.Context(), app.PriceObservationInput{
			OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
			BaseCommodityID: baseCommodityID, QuoteCommodityID: request.QuoteCommodityID, QuoteType: request.QuoteType,
			AdjustmentBasis: request.AdjustmentBasis, PriceValue: request.PriceValue, PriceScale: request.PriceScale,
			BaseQuantityValue: request.BaseQuantityValue, BaseQuantityScale: request.BaseQuantityScale, ValuationDate: valuationDate,
			ObservedAt: request.ObservedAt, SourcePublishedAt: request.SourcePublishedAt, SourceID: request.SourceID,
			MarketCode: request.MarketCode, ProviderSeriesID: request.ProviderSeriesID, ProviderObservationID: request.ProviderObservationID,
			IsManual: request.IsManual, IsDerived: request.IsDerived, SupersedesObservationID: request.SupersedesObservationID,
			DerivationJSON: rawJSONText(request.Derivation), SeriesMetadataJSON: rawJSONText(request.SeriesMetadata),
			MetadataJSON: rawJSONText(request.Metadata),
			ChangeReason: request.ChangeReason,
		})
		if err != nil {
			writePricingServiceError(w, r, logger, "create price", err)
			return
		}
		writeJSON(w, http.StatusCreated, toPriceObservationResponse(price))
	}))
}

type voidPriceRequest struct {
	VoidReason string `json:"void_reason"`
}

func voidPrice(logger *slog.Logger, authService *app.AuthService, pricingService *app.PricingService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		priceID, err := strconv.ParseInt(r.PathValue("price_id"), 10, 64)
		if err != nil || priceID <= 0 {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "price id is invalid")
			return
		}
		var request voidPriceRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		price, voidErr := pricingService.VoidPrice(r.Context(), app.VoidPriceInput{
			ObservationID: priceID,
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			VoidReason:    request.VoidReason,
		})
		if voidErr != nil {
			writePricingServiceError(w, r, logger, "void price", voidErr)
			return
		}
		writeJSON(w, http.StatusOK, toPriceObservationResponse(price))
	}))
}

func getPricingPolicy(logger *slog.Logger, authService *app.AuthService, pricingService *app.PricingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		policy, err := pricingService.GetPolicy(r.Context())
		if err != nil {
			writePricingServiceError(w, r, logger, "get pricing policy", err)
			return
		}
		writeJSON(w, http.StatusOK, toPricingPolicyResponse(policy))
	}
}

func savePricingPolicy(logger *slog.Logger, authService *app.AuthService, pricingService *app.PricingService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request pricingPolicyRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		policy, err := pricingService.SavePolicy(r.Context(), app.PricingPolicyInput{
			OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
			BaseCommodityID: request.BaseCommodityID, DefaultSourceID: request.DefaultSourceID, RefreshEnabled: request.RefreshEnabled,
			RefreshHourUTC: request.RefreshHourUTC, RefreshMinuteUTC: request.RefreshMinuteUTC, RefreshHourLocal: request.RefreshHourLocal,
			RefreshMinuteLocal: request.RefreshMinuteLocal, MaxBackfillDays: request.MaxBackfillDays,
			StalenessMaxDays: request.StalenessMaxDays, TriangulationMaxHops: request.TriangulationMaxHops,
			RoundingMode: request.RoundingMode, PreferOfficialFX: request.PreferOfficialFX, WeekendPolicy: request.WeekendPolicy,
			ChangeReason: request.ChangeReason,
		})
		if err != nil {
			writePricingServiceError(w, r, logger, "save pricing policy", err)
			return
		}
		writeJSON(w, http.StatusOK, toPricingPolicyResponse(policy))
	}))
}

func listPricingSourceAssignments(logger *slog.Logger, authService *app.AuthService, pricingService *app.PricingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		assignments, err := pricingService.ListSourceAssignments(r.Context())
		if err != nil {
			writePricingServiceError(w, r, logger, "list pricing source assignments", err)
			return
		}
		writeJSON(w, http.StatusOK, pricingSourceAssignmentsResponse{Assignments: toPricingSourceAssignmentResponses(assignments)})
	}
}

func savePricingSourceAssignment(logger *slog.Logger, authService *app.AuthService, pricingService *app.PricingService, options HandlerOptions, assignmentIDFromPath bool) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		assignmentID := int64(0)
		if assignmentIDFromPath {
			var parsedOK bool
			assignmentID, parsedOK = readPathInt64(w, r, "assignment_id", "assignment id")
			if !parsedOK {
				return
			}
		}
		var request pricingSourceAssignmentRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		assignment, err := pricingService.SaveSourceAssignment(r.Context(), app.PricingSourceAssignmentInput{
			OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
			AssignmentID: assignmentID, CommodityID: request.CommodityID, QuoteCommodityID: request.QuoteCommodityID,
			SourceID: request.SourceID, Priority: request.Priority, Status: request.Status, EffectiveFrom: request.EffectiveFrom,
			EffectiveTo: request.EffectiveTo, ChangeReason: request.ChangeReason,
		})
		if err != nil {
			writePricingServiceError(w, r, logger, "save pricing source assignment", err)
			return
		}
		status := http.StatusOK
		if !assignmentIDFromPath {
			status = http.StatusCreated
		}
		writeJSON(w, status, toPricingSourceAssignmentResponse(assignment))
	}))
}

func runPricingRefresh(logger *slog.Logger, authService *app.AuthService, pricingService *app.PricingService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request pricingRefreshRunRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		run, err := pricingService.RunRefresh(r.Context(), owner.ID, request.SourceID, request.Trigger)
		if err != nil {
			writePricingServiceError(w, r, logger, "run pricing refresh", err)
			return
		}
		writeJSON(w, http.StatusCreated, toPricingRefreshRunResponse(run))
	}))
}

func listPricingRefreshRuns(logger *slog.Logger, authService *app.AuthService, pricingService *app.PricingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		runs, err := pricingService.ListRefreshRuns(r.Context(), 50)
		if err != nil {
			writePricingServiceError(w, r, logger, "list pricing refresh runs", err)
			return
		}
		writeJSON(w, http.StatusOK, pricingRefreshRunsResponse{
			Runs:    toPricingRefreshRunResponses(runs.Runs),
			HasMore: runs.HasMore,
		})
	}
}

func pricingSourceHealth(logger *slog.Logger, authService *app.AuthService, pricingService *app.PricingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		sources, err := pricingService.SourceHealth(r.Context())
		if err != nil {
			writePricingServiceError(w, r, logger, "pricing source health", err)
			return
		}
		failedWork, err := pricingService.FailedBackgroundWork(r.Context())
		if err != nil {
			writePricingServiceError(w, r, logger, "pricing failed background work", err)
			return
		}
		writeJSON(w, http.StatusOK, pricingSourceHealthListResponse{
			Sources:              toPricingSourceHealthResponses(sources),
			FailedBackgroundWork: toPricingBackgroundWorkResponses(failedWork),
		})
	}
}

// retryPricingBackgroundWork re-enqueues one failed FX coverage work item. It
// is the manual escape hatch that makes the bounded-retry cap safe: work that
// gave up is not lost, it waits for the operator to fix the cause and ask for
// another run.
func retryPricingBackgroundWork(logger *slog.Logger, authService *app.AuthService, pricingService *app.PricingService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedMutationOwner(w, r); !ok {
			return
		}
		workID, ok := readPathInt64(w, r, "work_id", "background work id")
		if !ok {
			return
		}
		item, err := pricingService.RetryBackgroundWork(r.Context(), workID)
		if err != nil {
			writePricingServiceError(w, r, logger, "retry pricing background work", err)
			return
		}
		writeJSON(w, http.StatusOK, toPricingBackgroundWorkResponse(item))
	}))
}

func toPricingBackgroundWorkResponses(items []app.PricingBackgroundWorkItem) []pricingBackgroundWorkResponse {
	responses := make([]pricingBackgroundWorkResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, toPricingBackgroundWorkResponse(item))
	}
	return responses
}

func toPricingBackgroundWorkResponse(item app.PricingBackgroundWorkItem) pricingBackgroundWorkResponse {
	return pricingBackgroundWorkResponse{
		ID:        item.ID,
		Kind:      item.Kind,
		Reason:    item.Reason,
		Attempts:  item.Attempts,
		LastError: item.LastError,
		CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt,
	}
}

func writePricingServiceError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, action string, err error) {
	var validationError app.ValidationError
	switch {
	case errors.As(err, &validationError):
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", validationError.Error())
	case errors.Is(err, app.ErrPricingPolicyNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "pricing policy not found")
	case errors.Is(err, app.ErrPricingAssignmentNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "pricing source assignment not found")
	case errors.Is(err, app.ErrPricingBackgroundWorkNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "failed pricing background work not found")
	case errors.Is(err, app.ErrPriceObservationNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "price observation not found")
	case errors.Is(err, app.ErrPriceObservationAlreadyVoided):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "price observation is already voided")
	case errors.Is(err, app.ErrPricingBackgroundWorkActive):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "equivalent pricing background work is already queued")
	default:
		writeServiceInternalError(w, r, logger, action, err)
	}
}

func toMarketDataSourceResponses(sources []app.MarketDataSource) []marketDataSourceResponse {
	responses := make([]marketDataSourceResponse, 0, len(sources))
	for _, source := range sources {
		responses = append(responses, marketDataSourceResponse{ID: source.ID, Code: source.Code, Name: source.Name, Kind: source.Kind, ProviderKey: source.ProviderKey, BaseURL: source.BaseURL, Status: source.Status, Metadata: json.RawMessage(source.MetadataJSON), CreatedAt: source.CreatedAt})
	}
	return responses
}

func toPriceObservationResponse(price app.PriceObservation) priceObservationResponse {
	return priceObservationResponse{
		ID:                      price.ID,
		BookID:                  price.BookID,
		SeriesID:                price.SeriesID,
		BaseCommodityID:         price.BaseCommodityID,
		QuoteCommodityID:        price.QuoteCommodityID,
		QuoteType:               price.QuoteType,
		AdjustmentBasis:         price.AdjustmentBasis,
		PriceValue:              price.PriceValue,
		PriceScale:              price.PriceScale,
		BaseQuantityValue:       price.BaseQuantityValue,
		BaseQuantityScale:       price.BaseQuantityScale,
		ValuationDate:           price.ValuationDate,
		ObservedAt:              price.ObservedAt,
		SourcePublishedAt:       price.SourcePublishedAt,
		SourceID:                price.SourceID,
		ProviderObservationID:   price.ProviderObservationID,
		IsManual:                price.IsManual,
		IsDerived:               price.IsDerived,
		SupersedesObservationID: price.SupersedesObservationID,
		Derivation:              json.RawMessage(price.DerivationJSON),
		Metadata:                json.RawMessage(price.MetadataJSON),
		RecordedAt:              price.RecordedAt,
		VoidedAt:                price.VoidedAt,
		VoidReason:              price.VoidReason,
	}
}

func toPriceObservationResponses(prices []app.PriceObservation) []priceObservationResponse {
	responses := make([]priceObservationResponse, 0, len(prices))
	for _, price := range prices {
		responses = append(responses, toPriceObservationResponse(price))
	}
	return responses
}

func toPricingPolicyResponse(policy app.PricingPolicy) pricingPolicyResponse {
	return pricingPolicyResponse{ID: policy.ID, BookID: policy.BookID, BaseCommodityID: policy.BaseCommodityID, DefaultSourceID: policy.DefaultSourceID, RefreshEnabled: policy.RefreshEnabled, RefreshHourUTC: policy.RefreshHourUTC, RefreshMinuteUTC: policy.RefreshMinuteUTC, RefreshHourLocal: policy.RefreshHourLocal, RefreshMinuteLocal: policy.RefreshMinuteLocal, MaxBackfillDays: policy.MaxBackfillDays, StalenessMaxDays: policy.StalenessMaxDays, TriangulationMaxHops: policy.TriangulationMaxHops, RoundingMode: policy.RoundingMode, PreferOfficialFX: policy.PreferOfficialFX, WeekendPolicy: policy.WeekendPolicy, CreatedAt: policy.CreatedAt, UpdatedAt: policy.UpdatedAt}
}

func toPricingSourceAssignmentResponse(assignment app.PricingSourceAssignment) pricingSourceAssignmentResponse {
	return pricingSourceAssignmentResponse{ID: assignment.ID, BookID: assignment.BookID, CommodityID: assignment.CommodityID, QuoteCommodityID: assignment.QuoteCommodityID, SourceID: assignment.SourceID, Priority: assignment.Priority, Status: assignment.Status, EffectiveFrom: assignment.EffectiveFrom, EffectiveTo: assignment.EffectiveTo, CreatedAt: assignment.CreatedAt}
}

func toPricingSourceAssignmentResponses(assignments []app.PricingSourceAssignment) []pricingSourceAssignmentResponse {
	responses := make([]pricingSourceAssignmentResponse, 0, len(assignments))
	for _, assignment := range assignments {
		responses = append(responses, toPricingSourceAssignmentResponse(assignment))
	}
	return responses
}

func toPricingRefreshRunResponse(run app.PricingRefreshRun) pricingRefreshRunResponse {
	return pricingRefreshRunResponse{ID: run.ID, BookID: run.BookID, SourceID: run.SourceID, Trigger: run.Trigger, Status: run.Status, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt, ItemsTotal: run.ItemsTotal, ItemsSucceeded: run.ItemsSucceeded, ItemsFailed: run.ItemsFailed, LastError: run.LastError}
}

func toPricingRefreshRunResponses(runs []app.PricingRefreshRun) []pricingRefreshRunResponse {
	responses := make([]pricingRefreshRunResponse, 0, len(runs))
	for _, run := range runs {
		responses = append(responses, toPricingRefreshRunResponse(run))
	}
	return responses
}

func toPricingSourceHealthResponses(sources []app.PricingSourceHealth) []pricingSourceHealthResponse {
	responses := make([]pricingSourceHealthResponse, 0, len(sources))
	for _, source := range sources {
		responses = append(responses, pricingSourceHealthResponse{ID: source.ID, BookID: source.BookID, CommodityID: source.CommodityID, QuoteCommodityID: source.QuoteCommodityID, SourceID: source.SourceID, LastSuccessDate: source.LastSuccessDate, LastAttemptAt: source.LastAttemptAt, LastError: source.LastError, UpdatedAt: source.UpdatedAt, Status: source.Status})
	}
	return responses
}
